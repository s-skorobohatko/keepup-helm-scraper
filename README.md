# keepup-helm-scraper

## Helm Repository

```bash
helm repo add keepup-helm-scraper https://code-tool.github.io/keepup-helm-scraper/
```

Set mandatory variables
```yaml
env:
  CLUSTER_NAME: 'unique-name-for-metrics-labels'
  API_URL: 'https://keepup.host/helm-cluster'
  API_TOKEN: 'api-token-to-access-the-API_URL'
```

Deploy
```bash
helm install keepup-helm-scraper/keepup-helm-scraper
```
