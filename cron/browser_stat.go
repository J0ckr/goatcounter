// Copyright © 2019 Martin Tournoij <martin@arp242.net>
// This file is part of GoatCounter and published under the terms of the EUPL
// v1.2, which can be found in the LICENSE file or at http://eupl12.zgo.at

package cron

import (
	"context"
	"database/sql"
	"strings"

	"github.com/mssola/user_agent"
	"github.com/pkg/errors"
	"zgo.at/goatcounter"
	"zgo.at/zdb"
	"zgo.at/zdb/bulk"
)

// Browser are stored as a count per browser/version per day:
//
//  site |    day     | browser | version | count
// ------+------------+---------+---------+------
//     1 | 2019-12-17 | Chrome  | 38      |    13
//     1 | 2019-12-17 | Chrome  | 77      |     2
//     1 | 2019-12-17 | Opera   | 9       |     1
func updateBrowserStats(ctx context.Context, hits []goatcounter.Hit) error {
	return zdb.TX(ctx, func(ctx context.Context, tx zdb.DB) error {
		// Group by day + browser.
		type gt struct {
			count   int
			day     string
			browser string
			version string
		}
		grouped := map[string]gt{}
		for _, h := range hits {
			browser, version := getBrowser(h.Browser)
			if browser == "" {
				continue
			}

			day := h.CreatedAt.Format("2006-01-02")
			k := day + browser + " " + version
			v := grouped[k]
			if v.count == 0 {
				v.day = day
				v.browser = browser
				v.version = version
				var err error
				v.count, err = existingBrowserStats(ctx, tx, h.Site, day, v.browser, v.version)
				if err != nil {
					return err
				}
			}

			v.count += 1
			grouped[k] = v
		}

		siteID := goatcounter.MustGetSite(ctx).ID
		ins := bulk.NewInsert(ctx, tx,
			"browser_stats", []string{"site", "day", "browser", "version", "count"})
		for _, v := range grouped {
			ins.Values(siteID, v.day, v.browser, v.version, v.count)
		}
		return ins.Finish()
	})
}

func existingBrowserStats(
	txctx context.Context, tx zdb.DB, siteID int64,
	day, browser, version string,
) (int, error) {

	var c int
	err := tx.GetContext(txctx, &c,
		`select count from browser_stats where site=$1 and day=$2 and browser=$3 and version=$4`,
		siteID, day, browser, version)
	if err != nil && err != sql.ErrNoRows {
		return 0, errors.Wrap(err, "existing")
	}

	if err != sql.ErrNoRows {
		_, err = tx.ExecContext(txctx,
			`delete from browser_stats where site=$1 and day=$2 and browser=$3 and version=$4`,
			siteID, day, browser, version)
		if err != nil {
			return 0, errors.Wrap(err, "delete")
		}
	}

	return c, nil
}

func getBrowser(uaHeader string) (string, string) {
	ua := user_agent.New(uaHeader)
	browser, version := ua.Browser()

	// A lot of this is wrong, so just skip for now.
	if browser == "Android" {
		return "", ""
	}

	if browser == "Chromium" {
		browser = "Chrome"
	}

	// Correct some wrong data.
	if browser == "Safari" && strings.Count(version, ".") == 3 {
		browser = "Chrome"
	}
	// Note: Safari still shows Chrome and Firefox wrong.
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/User-Agent/Firefox
	// https://developer.chrome.com/multidevice/user-agent#chrome_for_ios_user_agent

	// The "build" and "patch" aren't interesting for us, and "minor" hasn't
	// been non-0 since 2010.
	// https://www.chromium.org/developers/version-numbers
	if browser == "Chrome" || browser == "Opera" {
		if i := strings.Index(version, "."); i > -1 {
			version = version[:i]
		}
	}

	// Don't include patch version.
	if browser == "Safari" {
		v := strings.Split(version, ".")
		if len(v) > 2 {
			version = v[0] + "." + v[1]
		}
	}

	return browser, version
}
