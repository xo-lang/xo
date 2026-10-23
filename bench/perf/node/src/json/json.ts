// Node twin of bench/perf/jsontwin: JSON.parse and JSON.stringify of 1000
// users, 100 rounds each. Usage: node dist/json/json.js decode|encode.
interface User {
  id: number;
  name: string;
  email: string;
  score: number;
  active: boolean;
  tags: string[];
}

const DOCS = 1000;
const ROUNDS = 100;

function users(): User[] {
  const us: User[] = [];
  let seed = 12345;
  for (let i = 0; i < DOCS; i++) {
    // the product passes 2^53: exact integer arithmetic, as in the other languages
    seed = Number((BigInt(seed) * 1103515245n + 12345n) % 2147483648n);
    const n = seed % 100000;
    const tags: string[] = [];
    for (let t = 0; t < seed % 5; t++) tags.push(`t${t}`);
    us.push({ id: n, name: `user ${n}`, email: `user_${n}@example.com`, score: (seed % 1000) + 0.5, active: seed % 2 === 0, tags });
  }
  return us;
}

// The document built by hand, not with the encoder under test.
function text(u: User): string {
  return `{"id":${u.id},"name":"${u.name}","email":"${u.email}","score":${u.score.toFixed(1)},"active":${u.active},"tags":[${u.tags.map((t) => `"${t}"`).join(",")}]}`;
}

const w = process.argv[2] ?? "decode";
const us = users();
let h = 0;
if (w === "decode") {
  const ds = us.map(text);
  for (let r = 0; r < ROUNDS; r++) {
    for (const d of ds) {
      const u = JSON.parse(d) as User;
      h = (h * 31 + u.id + u.name.length + u.email.length + u.tags.length + (Math.trunc(u.score) % 100) + (u.active ? 1 : 0)) % 1000000007;
    }
  }
} else if (w === "encode") {
  for (let r = 0; r < ROUNDS; r++) {
    for (const u of us) h = (h * 31 + Buffer.byteLength(JSON.stringify(u))) % 1000000007;
  }
} else {
  console.error("unknown workload", w);
  process.exit(2);
}
console.log(w, h);
