// xo-ci-register: called by a runner machine at boot (aws lambda invoke,
// allowed only to the xo-ci-runner role) with {name}. Returns whether
// GitHub still knows a runner of that name and a fresh registration
// token, so the App's key never lives on a machine.
import { gh, runners, REPO } from "./github.mjs";

export async function handler(event) {
  const name = String(event.name || "");
  if (!/^xo-ci-[a-z0-9-]+$/.test(name)) throw new Error("bad runner name: " + name);
  const r = (await runners()).find((x) => x.name === name);
  const reg = await gh("POST", `/repos/${REPO}/actions/runners/registration-token`);
  return { exists: !!r, status: r ? r.status : "absent", token: reg.token };
}
