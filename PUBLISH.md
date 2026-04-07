# Publishing mingledb-cli

## Release (GitHub + binaries)

1. **Ensure gomingleDB is on GitHub**  
   The release workflow checks out `mingledb/gomingleDB` and uses a local module replace during build. Ensure the `gomingleDB` repo exists under the `mingledb` org.

2. **Tag and push**
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

3. **What runs**
   - Workflow **Release** runs on push of tags `v*`.
   - It checks out **mingledb-cli** and **gomingleDB**, applies `go mod` replace to the checked-out dependency, builds the CLI for linux/windows/darwin (amd64 + arm64), and creates a **GitHub Release** with the binaries attached.

4. **Download**
   Users get the binaries from the **Releases** page of the mingledb-cli repo (e.g. `mgdb-linux-amd64.tar.gz`, `mgdb-windows-amd64.zip`, etc.).

---

## Local build (no CI)

With `gomingleDB` next to `mingledb-cli` (e.g. `../gomingleDB`):

```bash
cd mingledb-cli
go build -o mgdb .
```

On Windows:

```powershell
go build -o mgdb.exe .
```
