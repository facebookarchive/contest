# Schema migration
[tools/migration/rdbms](https://github.com/facebookincubator/contest/tree/master/tools/migration/rdbms) is a schema migration tool which supports defining incremental schema and data migrations directly via `.sql` files or via pure golang code. 

The two approaches serve different purposes: `.sql` migrations normally implement schema changes, while golang migrations are helpful to backfill data that needs to be manipulated in a way that might be specific to ConTest business logic. As en example of golang migration, please see [0002_migrate_descriptor_to_extended_descriptor.go](https://github.com/facebookincubator/contest/blob/master/db/rdbms/migration/0002_migrate_descriptor_to_extended_descriptor.go).


The tool tracks the schema version number and when migrations where applied. The current implementation relies on [pressly/goose](https://github.com/pressly/goose), library, but this might be bound to change in the future, still maintaining the same approach to migrations.

Migrations include an `up` and `down` commands: the former implements the forward migration, the latter implements the backward migration, i.e. rewinds the changes applied by the up migration. 
There are two type of migrations:
* Those defined in pure SQL, implemented in `.sql` files
* Those defined in pure golang, implemented in `go` files. The existence of `go` migrations supports the implementation of complex data manipulation which is not suitable for for `sql`

All migrations are numbered and applied sequentially. Examples of the above can be found in the [migration directory](https://github.com/facebookincubator/contest/tree/master/db/rdbms/migration).

# Usage
The tool supports the following commands:


```
Commands:
    up                   Migrate the DB to the most recent version available
    up-by-one            Migrate the DB up by 1
    up-to VERSION        Migrate the DB to a specific VERSION
    down                 Roll back the version by 1
    down-to VERSION      Roll back to a specific VERSION
    redo                 Re-run the latest migration
    reset                Roll back all migrations
    status               Dump the migration status for the current DB
    version              Print the current version of the database
    create NAME [sql|go] Creates new migration file with the current timestamp
    fix                  Apply sequential ordering to migrations
```
The most commonly used are `up`, `down`, `status`.

In addition to these commands, additional flags are supported by the tool:
```
Usage: migrate [OPTIONS] COMMAND
  -dbDriver string
        DB Driver (default "mysql")
  -dbURI string
        Database URI (default "contest:contest@tcp(localhost:3306)/contest?parseTime=true")
  -debug
        Enabled debug logging
  -dir string
        Directory containing migration scripts
```
`-dir` is especially relevant as it points to the directory containing `.sql` migrations. These are part of ConTest codebase and can be found in [db/rdbms/migration](https://github.com/facebookincubator/contest/tree/master/db/rdbms/migration). Please see that directory for an explanation of what those migrations actually do

# Status command
`status` can be used to inspect the progress in applying all migrations known to ConTest. For example, assuming we have the following two migrations in the codebase:
* `0001_add_extended_descriptor.sql`
* `0002_migrate_descriptor_to_extended_descriptor.go`


The output of `status` will looks as follows:
```
go run tools/migration/rdbms/main.go -dir db/rdbms/migration status 
2020/12/09 19:02:45     Applied At                  Migration
2020/12/09 19:02:45     =======================================
2020/12/09 19:02:45     Pending                  -- 0001_add_extended_descriptor.sql
2020/12/09 19:02:45     Pending                  -- 0002_migrate_descriptor_to_extended_descriptor.go
```

We can now proceed applying the migrations.

# Up command
`up` command, and its variations, can be used to run forward migrations. Taking for example the output of the `status` command above, we can run both migrations in sequence:

```
go run tools/migration/rdbms/main.go -dir db/rdbms/migration up
2020/12/09 19:11:04 OK    0001_add_extended_descriptor.sql
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering target manager csvfiletargetmanager
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering target manager targetlist
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test fetcher uri
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test fetcher literal
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test step echo
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test step slowecho
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test step example
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test step cmd
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test step sshcmd
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering test step randecho
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering reporter targetsuccess
[2020-12-09T19:11:04Z]  INFO pkg/pluginregistry: Registering reporter noop
2020/12/09 19:11:04 OK    0002_migrate_descriptor_to_extended_descriptor.go
2020/12/09 19:11:04 goose: no migrations to run. current version: 2
```

The tool indicates that the version of the schema has been brought up to `2`. Output of `status` confirms this:
```
 $ go run tools/migration/rdbms/main.go -dir db/rdbms/migration status
2020/12/09 19:12:05     Applied At                  Migration
2020/12/09 19:12:05     =======================================
2020/12/09 19:12:05     Wed Dec  9 19:11:04 2020 -- 0001_add_extended_descriptor.sql
2020/12/09 19:12:05     Wed Dec  9 19:11:04 2020 -- 0002_migrate_descriptor_to_extended_descriptor.go
```

# Down command
`down` is the counterpart of `up`, as it rewinds the migration. Note that what actually happens when a migration is executed on the `down` path is very dependent on the implementation of the migration itself. `down` migrations might be a no-op, in which case the tool will only roll back the version, but no actual work will be executed. 
