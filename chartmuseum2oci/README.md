# Chartmuseum2OCI

[Harbor](https://github.com/goharbor/harbor) supports two different ways to store the Helms charts data:
    1. stored in Harbor registry storage directly via OCI API.
    2. stored in Harbor hosted chartmuseum backend via chartmuseum's API.

From Harbor 2.6, [Chartmuseum](https://github.com/helm/chartmuseum) is deprecated and is removed from Harbor 2.8.

`Chartmuseum 2 OCI` tool purpose is to migrate Helm charts from Harbor Chartmuseum to Helm OCI registry.
It copies Helm charts but don't delete them from Chartmuseum.

## Requirements

- Docker

## Build

```bash
docker build -t goharbor/chartmuseum2oci .
```

## Usage

```bash
docker run -ti --rm goharbor/chartmuseum2oci --url $HARBOR_URL --username $HARBOR_USER --password $HARBOR_PASSWORD
```

Note: Harbor by default has a hardcoded page size of 10, so the above command only works for the first 10 projects.
To overcome this issue, the parameters `--page` and `--pagesize` (max. 100) can be added.

Example:
```bash
docker run -ti --rm goharbor/chartmuseum2oci --url $HARBOR_URL --username $HARBOR_USER --password $HARBOR_PASSWORD --page 2 --pagesize 50
```
