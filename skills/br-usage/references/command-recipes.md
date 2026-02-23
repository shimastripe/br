# br Command Recipes

## Authentication

```bash
br auth login --with-token <TOKEN>
br auth login --with-token < token.txt
br auth status
```

## App Discovery

```bash
br application app-list
br application app-show --app-slug <app-slug>
```

## Builds

```bash
br builds list --app-slug <app-slug>
br builds list --app-slug <app-slug> --format json
br builds list --app-slug <app-slug> --format json --fields slug,status,triggered_workflow
br builds show --app-slug <app-slug> --build-slug <build-slug>
br builds trigger --app-slug <app-slug> -f branch=main
br builds log --app-slug <app-slug> --build-slug <build-slug>
```

## Pipelines

```bash
br pipelines list --app-slug <app-slug>
br pipelines show --app-slug <app-slug> --pipeline-id <pipeline-id>
br pipelines abort --app-slug <app-slug> --pipeline-id <pipeline-id>
```

## Formatting and Filtering

```bash
# table is default for generated GET commands
br builds list --app-slug <app-slug>

# switch to JSON for scripting
br builds list --app-slug <app-slug> --format json

# pick specific fields
br builds list --app-slug <app-slug> --format json --fields slug,status

# template rendering (requires --format json + --fields)
br builds list --app-slug <app-slug> --format json --fields slug,status \
  --template '{{range .}}{{.slug}} {{.status}}{{"\n"}}{{end}}'
```

## Generic API Fallback

```bash
br api /apps/<app-slug>/builds -X GET
br api /apps/<app-slug>/builds -X POST -f branch=main
br api /apps/<app-slug>/builds --paginate --slurp
```
