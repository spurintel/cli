# deployments/

When this project is ready for deployment, create the relevant subdirectories here.

## Conventions

- `pulumi/` — cloud infrastructure (networks, DNS, managed services, Kubernetes resources).
  **Pulumi projects must use Go.** Initialize with: `cd deployments && pulumi new <provider>-go` (e.g. `gcp-go`, `aws-go`).
- `ansible/` — machine setup (packages, services, files on VMs).
  Initialize with: `mkdir -p deployments/ansible && cd deployments/ansible && ansible-galaxy init roles/<role-name>`.

## Order

Pulumi provisions first, Ansible configures second. Pulumi does not invoke Ansible automatically — document the sequence in your deploy runbook.
