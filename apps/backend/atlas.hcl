data "external_schema" "ent" {
  program = [
    "go",
    "run",
    "-mod=mod",
    "entgo.io/ent/cmd/ent",
    "describe",
    "./ent/schema"
  ]
}

env "local" {
  src = data.external_schema.ent.url
  url = getenv("POSTGRES_DSN")
  dev = getenv("POSTGRES_DEV_DSN")
  migration {
    dir = "file://ent/migrate/migrations"
  }
  format {
    migrate {
      diff = "{{ .Version }}_{{ .Name }}.sql"
    }
  }
}
