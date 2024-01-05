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

### Project filtering

Using the option `--project` (can be specified multiple times), the migration can be limited to only a particular set of projects, instead of the default behaviour, which is "all at once".

```bash
docker run -ti --rm goharbor/chartmuseum2oci --url $HARBOR_URL --username $HARBOR_USER --password $HARBOR_PASSWORD --project pr1 --project pr2
```

### Destination path

Using the option `--destpath` a subpath within the project can be specified, in which the charts will be pushed.

In this example, the charts will be pushed into `$HARBOR_URL/$PROJECT/charts`:
```bash
docker run -ti --rm goharbor/chartmuseum2oci --url $HARBOR_URL --username $HARBOR_USER --password $HARBOR_PASSWORD --destpath /charts
```
