# infra/state: the Terraform state bucket

The private S3 bucket `xo-tfstate-ACCOUNT_ID` (us-east-1) that holds the
Terraform state of `infra/ci-runners` (decision 0225,
`knowledgebase/infrastructure/ci.md`) and of this folder:
versioned (old versions expire after 90 days), public access blocked,
AES256 encryption, `prevent_destroy`. Locking is S3's own lock file
(`use_lockfile`, Terraform 1.10 or later), no DynamoDB table.

State keys: `state/terraform.tfstate` (this folder),
`ci-runners/terraform.tfstate` (infra/ci-runners).

Created 2026-10-14 with local state, then moved into the bucket:

```
cd infra/state
terraform init && terraform apply         # without backend.tf, first time only
# add backend.tf
terraform init -migrate-state -force-copy
```

Day to day: `terraform init && terraform plan` (expect no changes).
