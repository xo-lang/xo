// xo-ci-autostart: starts stopped runner machines for queued jobs.
// Invoked by GitHub's workflow_job webhook (function URL, HMAC checked
// against /xo-ci/webhook-secret) and every minute by a schedule, which
// catches jobs whose webhook was missed or whose machine was still
// stopping. Machines stop themselves when idle (boot/linux.sh).
import { createHmac, timingSafeEqual } from "node:crypto";
import { EC2Client, DescribeInstancesCommand, StartInstancesCommand } from "@aws-sdk/client-ec2";
import { gh, runners, param, REPO } from "./github.mjs";

const ec2 = new EC2Client({});

function verify(event) {
  const body = event.isBase64Encoded ? Buffer.from(event.body, "base64") : Buffer.from(event.body || "");
  const sig = (event.headers || {})["x-hub-signature-256"] || "";
  return param("/xo-ci/webhook-secret").then((secret) => {
    const want = Buffer.from("sha256=" + createHmac("sha256", secret).update(body).digest("hex"));
    const got = Buffer.from(sig);
    if (got.length !== want.length || !timingSafeEqual(got, want)) return null;
    return JSON.parse(body.toString());
  });
}

const ours = (labels) => labels.filter((l) => l.startsWith("xo-"));

// Queued jobs of the repository, as a count per xo-* label set.
async function queuedJobs() {
  const jobs = [];
  for (const status of ["queued", "in_progress"]) {
    const r = await gh("GET", `/repos/${REPO}/actions/runs?status=${status}&per_page=50`);
    for (const run of r.workflow_runs) {
      const j = await gh("GET", `/repos/${REPO}/actions/runs/${run.id}/jobs?filter=latest&per_page=100`);
      for (const job of j.jobs) if (job.status === "queued" && ours(job.labels).length) jobs.push(ours(job.labels));
    }
  }
  return jobs;
}

async function machines() {
  const r = await ec2.send(new DescribeInstancesCommand({
    Filters: [{ Name: "tag:purpose", Values: ["xo-ci"] }, { Name: "instance-state-name", Values: ["pending", "running", "stopping", "stopped"] }],
  }));
  const tag = (i, k) => (i.Tags.find((t) => t.Key === k) || {}).Value || "";
  return r.Reservations.flatMap((x) => x.Instances).map((i) => ({
    id: i.InstanceId, state: i.State.Name, type: i.InstanceType,
    runner: tag(i, "ci-runner"), labels: tag(i, "ci-labels").split(",").filter(Boolean),
  }));
}

// For each queued job, find capacity that will take it: an idle online
// runner, or a machine already booting; otherwise start a stopped
// machine carrying all of the job's labels, the one with the fewest
// labels first (the dedicated small machines before the big ones).
async function reconcile(jobs) {
  if (!jobs.length) return [];
  const [rs, ms] = await Promise.all([runners(), machines()]);
  const online = new Map(rs.map((r) => [r.name, r]));
  const fits = (m, labels) => labels.every((l) => m.labels.includes(l));
  const free = ms.filter((m) => {
    const r = online.get(m.runner);
    return m.state === "pending" || (m.state === "running" && (!r || r.status !== "online" || !r.busy));
  });
  const stopped = ms.filter((m) => m.state === "stopped").sort((a, b) => a.labels.length - b.labels.length);
  const start = [];
  for (const labels of jobs) {
    let i = free.findIndex((m) => fits(m, labels));
    if (i >= 0) { free.splice(i, 1); continue; }
    i = stopped.findIndex((m) => fits(m, labels));
    if (i >= 0) start.push(stopped.splice(i, 1)[0].id);
  }
  if (start.length) await ec2.send(new StartInstancesCommand({ InstanceIds: start }));
  console.log(JSON.stringify({ jobs, start }));
  return start;
}

export async function handler(event) {
  if (event.source === "aws.scheduler" || event.sweep) {
    return { started: await reconcile(await queuedJobs()) };
  }
  const payload = await verify(event);
  if (!payload) return { statusCode: 401, body: "bad signature" };
  if (payload.action !== "queued" || !payload.workflow_job) return { statusCode: 200, body: "ignored" };
  // The webhook names one job; reconcile all queued jobs so concurrent
  // webhooks do not each start a machine for the same work.
  const started = await reconcile(await queuedJobs());
  return { statusCode: 200, body: JSON.stringify({ started }) };
}
