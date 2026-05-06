# Helm Chart Release Process

## Publishing a New Version

1. **Update Chart Version**
   ```bash
   # Edit deploy/helm/simple-local-path-provisioner/Chart.yaml
   version: 0.2.0         # Increment chart version
   appVersion: "0.2.0"    # Update app version
   ```

2. **Package and Generate Index**
   ```bash
   helm package deploy/helm/simple-local-path-provisioner/ -d helm-charts/packages/
   helm repo index helm-charts/ --url https://ursweiss.github.io/simple-local-path-provisioner/
   ```

3. **Commit and Push**
   ```bash
   git add helm-charts/ deploy/helm/simple-local-path-provisioner/Chart.yaml
   git commit -m "Release v0.2.0"
   git push origin main
   ```

4. **Verify**
   - Check GitHub Pages deployment in **Settings → Pages**
   - Test: `helm repo update && helm search repo simple-local-path-provisioner`

## Adding to Helm Repository

```bash
helm repo add simple-local-path-provisioner https://ursweiss.github.io/simple-local-path-provisioner/
helm install provisioner simple-local-path-provisioner/simple-local-path-provisioner
```
