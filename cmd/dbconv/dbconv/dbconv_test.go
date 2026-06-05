// Copyright 2023-2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md
package dbconv

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"

	"github.com/offchainlabs/nitro/util/testhelpers"
)

type kvFixture struct {
	key   []byte
	value []byte
}

var conversionFixtures = []kvFixture{
	{key: []byte{}, value: []byte{0xde, 0xed, 0xbe, 0xef}},
	{key: []byte{0x00}, value: nil},
	{key: []byte{0x00, 0x00}, value: []byte{}},
	{key: []byte{0x00, 0xff}, value: []byte{0x10}},
	{key: []byte{0x01}, value: []byte{0x02}},
	{key: []byte("prefix"), value: []byte("base")},
	{key: []byte("prefix\x00"), value: []byte("child-zero")},
	{key: []byte("prefix/a"), value: []byte("child-a")},
	{key: []byte("prefix/aa"), value: []byte("child-aa")},
	{key: []byte("prefix/b"), value: []byte{}},
	{key: []byte{0x7f, 0x00, 0xff, 0x10}, value: []byte{0x42, 0x00, 0xff}},
	{key: []byte{0xff}, value: []byte{0x00, 0xff}},
	{key: []byte{0xff, 0x00}, value: []byte{0xde}},
}

func TestConversion(t *testing.T) {
	tests := []struct {
		name      string
		srcEngine string
		dstEngine string
	}{
		{name: "LevelDBToPebble", srcEngine: rawdb.DBLeveldb, dstEngine: rawdb.DBPebble},
		{name: "PebbleToTreeDB", srcEngine: rawdb.DBPebble, dstEngine: rawdb.DBTreedb},
		{name: "LevelDBToTreeDB", srcEngine: rawdb.DBLeveldb, dstEngine: rawdb.DBTreedb},
		{name: "TreeDBToPebble", srcEngine: rawdb.DBTreedb, dstEngine: rawdb.DBPebble},
		{name: "TreeDBToLevelDB", srcEngine: rawdb.DBTreedb, dstEngine: rawdb.DBLeveldb},
		{name: "TreeDBToTreeDB", srcEngine: rawdb.DBTreedb, dstEngine: rawdb.DBTreedb},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldDBConfig := testDBConfig(t, test.srcEngine, "src")
			newDBConfig := testDBConfig(t, test.dstEngine, "dst")

			putFixtures(t, oldDBConfig, conversionFixtures)

			config := DefaultDBConvConfig
			config.Src = oldDBConfig
			config.Dst = newDBConfig
			config.IdealBatchSize = 5
			config.Verify = "full"
			conv := NewDBConverter(&config)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := conv.Convert(ctx)
			Require(t, err)

			err = conv.Verify(ctx)
			Require(t, err)

			assertDBMatchesFixtures(t, newDBConfig, conversionFixtures)
		})
	}
}

func TestVerifyFullReportsMissingDestinationKey(t *testing.T) {
	srcConfig := testDBConfig(t, rawdb.DBLeveldb, "src")
	dstConfig := testDBConfig(t, rawdb.DBTreedb, "dst")

	putFixtures(t, srcConfig, conversionFixtures[:1])
	putFixtures(t, dstConfig, nil)

	config := DefaultDBConvConfig
	config.Src = srcConfig
	config.Dst = dstConfig
	config.Verify = "full"
	conv := NewDBConverter(&config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := conv.Verify(ctx)
	if err == nil {
		Fail(t, "expected verification to fail on a missing destination key")
	}
	if !strings.Contains(err.Error(), "Missing key in destination db") {
		Fail(t, "unexpected verification error:", err)
	}
}

func TestVerifyFullReportsExtraDestinationKey(t *testing.T) {
	srcConfig := testDBConfig(t, rawdb.DBLeveldb, "src")
	dstConfig := testDBConfig(t, rawdb.DBTreedb, "dst")

	putFixtures(t, srcConfig, conversionFixtures[:1])
	extraFixtures := append([]kvFixture(nil), conversionFixtures[:1]...)
	extraFixtures = append(extraFixtures, kvFixture{key: []byte("extra"), value: []byte("value")})
	putFixtures(t, dstConfig, extraFixtures)

	config := DefaultDBConvConfig
	config.Src = srcConfig
	config.Dst = dstConfig
	config.Verify = "full"
	conv := NewDBConverter(&config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := conv.Verify(ctx)
	if err == nil {
		Fail(t, "expected verification to fail on an extra destination key")
	}
	if !strings.Contains(err.Error(), "Unexpected key in destination db") {
		Fail(t, "unexpected verification error:", err)
	}
}

func TestDBConvConfigValidateAllowsTreeDB(t *testing.T) {
	config := DefaultDBConvConfig
	config.Src.DBEngine = rawdb.DBTreedb
	config.Dst.DBEngine = rawdb.DBTreedb
	config.Convert = true
	err := config.Validate()
	Require(t, err)
}

func TestDBConvConfigValidateRejectsUnknownEngine(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*DBConvConfig)
		want      string
	}{
		{
			name: "source",
			configure: func(config *DBConvConfig) {
				config.Src.DBEngine = "mssql"
			},
			want: "invalid src.db-engine choice",
		},
		{
			name: "destination",
			configure: func(config *DBConvConfig) {
				config.Dst.DBEngine = "mssql"
			},
			want: "invalid dst.db-engine choice",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultDBConvConfig
			config.Convert = true
			test.configure(&config)
			err := config.Validate()
			if err == nil {
				Fail(t, "expected invalid db engine error")
			}
			for _, want := range []string{test.want, `allowed "leveldb", "pebble", "treedb" or ""`} {
				if got := err.Error(); !strings.Contains(got, want) {
					Fail(t, "Validate error =", got, "want substring", want)
				}
			}
		})
	}
}

func testDBConfig(t *testing.T, engine string, namespace string) DBConfig {
	t.Helper()
	config := DBConfigDefaultDst
	config.Data = t.TempDir()
	config.DBEngine = engine
	config.Namespace = namespace + "/"
	return config
}

func putFixtures(t *testing.T, config DBConfig, fixtures []kvFixture) {
	t.Helper()
	db, err := openDB(&config, "", false)
	Require(t, err)
	defer func() {
		Require(t, db.Close())
	}()

	for _, fixture := range fixtures {
		err = db.Put(fixture.key, fixture.value)
		Require(t, err)
	}
}

func assertDBMatchesFixtures(t *testing.T, config DBConfig, fixtures []kvFixture) {
	t.Helper()
	db, err := openDB(&config, "", true)
	Require(t, err)
	defer func() {
		Require(t, db.Close())
	}()

	expected := make(map[string][]byte, len(fixtures))
	for _, fixture := range fixtures {
		expected[string(fixture.key)] = append([]byte(nil), fixture.value...)
	}

	seen := make(map[string]struct{}, len(fixtures))
	it := db.NewIterator(nil, nil)
	defer it.Release()
	var (
		prev     []byte
		havePrev bool
	)
	for it.Next() {
		key := append([]byte(nil), it.Key()...)
		if havePrev && bytes.Compare(prev, key) >= 0 {
			Fail(t, "Iterator returned keys out of order, previous:", prev, "current:", key)
		}
		prev = key
		havePrev = true
		want, ok := expected[string(key)]
		if !ok {
			Fail(t, "Unexpected key in the converted db, key:", key)
		}
		got := append([]byte(nil), it.Value()...)
		if len(got) != len(want) || !bytes.Equal(got, want) {
			Fail(t, "Value mismatch for key:", key, "got:", got, "want:", want)
		}
		seen[string(key)] = struct{}{}
	}
	Require(t, it.Error())

	for _, fixture := range fixtures {
		if _, ok := seen[string(fixture.key)]; !ok {
			Fail(t, "Missing key in the converted db, key:", fixture.key)
		}
	}
}

func Require(t *testing.T, err error, printables ...interface{}) {
	t.Helper()
	testhelpers.RequireImpl(t, err, printables...)
}

func Fail(t *testing.T, printables ...interface{}) {
	t.Helper()
	testhelpers.FailImpl(t, printables...)
}
