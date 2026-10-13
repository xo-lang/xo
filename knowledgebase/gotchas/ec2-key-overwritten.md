# The xo-ci SSH key was overwritten with nothing

Status: Observed (2026-10-13), fixed.

Symptom: every EC2 run failed after setup with `Load key
"~/.ssh/xo-ci.pem": invalid format`; the key file was 0 bytes.

Cause: an agent ran scripts/remote-full.sh inside a sandbox that could
not reach the AWS API. `aws ec2 describe-key-pairs --key-names xo-ci`
failed for a network reason, the script read any failure as "the key
pair does not exist", and ran `aws ec2 create-key-pair ... > "$key"`,
whose failure left the redirect's truncated file in place of the real
private key. The key pair on AWS still held the old public key, so the
private key was lost.

Fix: the lead deleted and recreated the xo-ci key pair (writing the new
key through a temporary file). remote-full.sh now creates the key pair
only when AWS answers InvalidKeyPair.NotFound, never overwrites an
existing key file, writes the new key through a temporary file checked
for a PEM header, and stops early if the key file is empty. Agents' AWS
runs are allowed outside the sandbox in the project's local settings
(owner, 2026-10-13), so the sandbox no longer intercepts them.
