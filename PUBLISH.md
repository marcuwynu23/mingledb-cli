# Publishing mingledb-cli

## Release (GitHub + binaries)

1. **Ensure gomingleDB is on GitHub**  
   The release workflow expects a repo named `gomingleDB` under the **same owner** as `mingledb-cli` (e.g. `marcuwynu23/gomingleDB`). If your gomingleDB repo is elsewhere, edit `.github/workflows/release.yml` and set the `repository` in the "Checkout gomingleDB" step.

2. **Tag and push**
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

3. **What runs**
   - Workflow **Release** runs on push of tags `v*`.
   - It checks out **mingledb-cli** and **gomingleDB** (sibling), builds the CLI for linux/windows/darwin (amd64 + arm64), and creates a **GitHub Release** with the binaries attached.

4. **Download**
   Users get the binaries from the **Releases** page of the mingledb-cli repo (e.g. `mingledb-cli-linux-amd64.tar.gz`, `mingledb-cli-windows-amd64.zip`, etc.).

---

## Local build (no CI)

With gomingleDB next to mingledb-cli (e.g. `../gomingleDB`):

```bash
cd mingledb-cli
go build -o mingledb-cli .
```

On Windows:

```powershell
go build -o mingledb-cli.exe .
```
