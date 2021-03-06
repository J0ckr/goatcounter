// Copyright © 2019 Martin Tournoij <martin@arp242.net>
// This file is part of GoatCounter and published under the terms of the EUPL
// v1.2, which can be found in the LICENSE file or at http://eupl12.zgo.at

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"zgo.at/goatcounter/cfg"
	"zgo.at/goatcounter/pack"
	"zgo.at/utils/sliceutil"
	"zgo.at/zdb"
	"zgo.at/zlog"
)

const usageMigrate = `
Run database migrations and exit.

Flags:

  -db            Database connection string. Use "sqlite://<dbfile>" for SQLite,
                 or "postgres://<connect string>" for PostgreSQL
                 Default: sqlite://db/goatcounter.sqlite3

  -debug         Modules to debug, comma-separated or 'all' for all modules.

Positional argumts are names of database migrations, either as just the name
("2020-01-05-2-foo") or as the file path ("./db/migrate/sqlite/2020-01-05-2-foo.sql").

Use "all" to run all migrations that haven't been run yet, or "show" to only
display pending migrations.

Note: you can also use -automigrate flag for the serve and saas commands to run
migrations on startup.
`

func migrate() (int, error) {
	if len(os.Args) == 2 {
		return 1, errors.New("need a migration or command")
	}

	dbConnect := flagDB()
	debug := flagDebug()
	CommandLine.Parse(os.Args[2:])
	zlog.Config.SetDebug(*debug)

	db, err := connectDB(*dbConnect, CommandLine.Args(), false)
	if err != nil {
		return 2, err
	}
	defer db.Close()

	if sliceutil.InStringSlice(CommandLine.Args(), "show") {
		m := zdb.NewMigrate(db, []string{"show"},
			map[bool]map[string][]byte{true: pack.MigrationsPgSQL, false: pack.MigrationsSQLite}[cfg.PgSQL],
			map[bool]string{true: "db/migrate/pgsql", false: "db/migrate/sqlite"}[cfg.PgSQL])
		have, ran, err := m.List()
		if err != nil {
			return 1, err
		}
		if d := sliceutil.DifferenceString(have, ran); len(d) > 0 {
			fmt.Fprintf(stdout, "Pending migrations:\n\t%s\n", strings.Join(d, "\n\t"))
		} else {
			fmt.Fprintln(stdout, "No pending migrations")
		}
		if d := sliceutil.DifferenceString(ran, have); len(d) > 0 {
			fmt.Fprintf(stdout, "Migrations in the DB that don't exist:\n\t%s\n", strings.Join(d, "\n\t"))
		}
	}

	return 0, nil
}
