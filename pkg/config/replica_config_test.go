// Copyright 2021 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"bytes"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mustIndentJSON(t *testing.T, j string) string {
	var buf bytes.Buffer
	err := json.Indent(&buf, []byte(j), "", "  ")
	require.Nil(t, err)
	return buf.String()
}

func TestReplicaConfigMarshal(t *testing.T) {
	t.Parallel()
	conf := GetDefaultReplicaConfig()
	conf.CaseSensitive = false
	conf.ForceReplicate = true
	conf.Filter.Rules = []string{"1.1"}
	conf.Mounter.WorkerNum = 3
	conf.Sink.Protocol = "open-protocol"
	conf.Sink.ColumnSelectors = []*ColumnSelector{
		{
			Matcher: []string{"1.1"},
			Columns: []string{"a", "b"},
		},
	}
	conf.Sink.CSVConfig = &CSVConfig{
		Delimiter:       ",",
		Quote:           "\"",
		NullString:      `\N`,
		IncludeCommitTs: true,
	}
	conf.Sink.Terminator = ""
	conf.Sink.DateSeparator = "month"
	conf.Sink.EnablePartitionSeparator = true
	conf.Sink.EnableKafkaSinkV2 = true
	conf.Scheduler.EnableTableAcrossNodes = true
	conf.Scheduler.RegionThreshold = 100001
	conf.Scheduler.WriteKeyThreshold = 100001

	b, err := conf.Marshal()
	require.Nil(t, err)
	require.JSONEq(t, testCfgTestReplicaConfigMarshal1, mustIndentJSON(t, b))
	conf2 := new(ReplicaConfig)
	err = conf2.UnmarshalJSON([]byte(testCfgTestReplicaConfigMarshal2))
	require.Nil(t, err)
	require.Equal(t, conf, conf2)
}

func TestReplicaConfigClone(t *testing.T) {
	t.Parallel()
	conf := GetDefaultReplicaConfig()
	conf.CaseSensitive = false
	conf.ForceReplicate = true
	conf.Filter.Rules = []string{"1.1"}
	conf.Mounter.WorkerNum = 3
	conf2 := conf.Clone()
	require.Equal(t, conf, conf2)
	conf2.Mounter.WorkerNum = 4
	require.Equal(t, 3, conf.Mounter.WorkerNum)
}

func TestReplicaConfigOutDated(t *testing.T) {
	t.Parallel()
	conf2 := new(ReplicaConfig)
	err := conf2.UnmarshalJSON([]byte(testCfgTestReplicaConfigOutDated))
	require.Nil(t, err)

	conf := GetDefaultReplicaConfig()
	conf.CaseSensitive = false
	conf.ForceReplicate = true
	conf.Filter.Rules = []string{"1.1"}
	conf.Mounter.WorkerNum = 3
	conf.Sink.Protocol = "open-protocol"
	conf.Sink.DispatchRules = []*DispatchRule{
		{Matcher: []string{"a.b"}, DispatcherRule: "r1"},
		{Matcher: []string{"a.c"}, DispatcherRule: "r2"},
		{Matcher: []string{"a.d"}, DispatcherRule: "r2"},
	}
	conf.Sink.TxnAtomicity = unknownTxnAtomicity
	conf.Sink.DateSeparator = ""
	conf.Sink.CSVConfig = nil
	require.Equal(t, conf, conf2)
}

func TestReplicaConfigValidate(t *testing.T) {
	t.Parallel()
	conf := GetDefaultReplicaConfig()

	sinkURL, err := url.Parse("blackhole://")
	require.NoError(t, err)

	require.Nil(t, conf.ValidateAndAdjust(sinkURL))

	// Incorrect sink configuration.
	conf = GetDefaultReplicaConfig()
	conf.Sink.Protocol = "canal"
	conf.EnableOldValue = false
	require.Regexp(t, ".*canal protocol requires old value to be enabled.*",
		conf.ValidateAndAdjust(sinkURL))

	conf = GetDefaultReplicaConfig()
	conf.Sink.DispatchRules = []*DispatchRule{
		{Matcher: []string{"a.b"}, DispatcherRule: "d1", PartitionRule: "r1"},
	}
	require.Regexp(t, ".*dispatcher and partition cannot be configured both.*",
		conf.ValidateAndAdjust(sinkURL))

	// Correct sink configuration.
	conf = GetDefaultReplicaConfig()
	conf.Sink.DispatchRules = []*DispatchRule{
		{Matcher: []string{"a.b"}, DispatcherRule: "d1"},
		{Matcher: []string{"a.c"}, PartitionRule: "p1"},
		{Matcher: []string{"a.d"}},
	}
	err = conf.ValidateAndAdjust(sinkURL)
	require.NoError(t, err)
	rules := conf.Sink.DispatchRules
	require.Equal(t, "d1", rules[0].PartitionRule)
	require.Equal(t, "p1", rules[1].PartitionRule)
	require.Equal(t, "", rules[2].PartitionRule)

	// Test memory quota can be adjusted
	conf = GetDefaultReplicaConfig()
	conf.MemoryQuota = 0
	err = conf.ValidateAndAdjust(sinkURL)
	require.NoError(t, err)
	require.Equal(t, uint64(DefaultChangefeedMemoryQuota), conf.MemoryQuota)

	conf.MemoryQuota = uint64(1024)
	err = conf.ValidateAndAdjust(sinkURL)
	require.NoError(t, err)
	require.Equal(t, uint64(1024), conf.MemoryQuota)
}

func TestValidateAndAdjust(t *testing.T) {
	cfg := GetDefaultReplicaConfig()
	require.False(t, cfg.EnableSyncPoint)

	sinkURL, err := url.Parse("blackhole://")
	require.NoError(t, err)

	require.NoError(t, cfg.ValidateAndAdjust(sinkURL))

	cfg.EnableSyncPoint = true
	require.NoError(t, cfg.ValidateAndAdjust(sinkURL))

	cfg.SyncPointInterval = time.Second * 29
	require.Error(t, cfg.ValidateAndAdjust(sinkURL))

	cfg.SyncPointInterval = time.Second * 30
	cfg.SyncPointRetention = time.Minute * 10
	require.Error(t, cfg.ValidateAndAdjust(sinkURL))

	cfg.Sink.EncoderConcurrency = -1
	require.Error(t, cfg.ValidateAndAdjust(sinkURL))

	cfg = GetDefaultReplicaConfig()
	cfg.Scheduler = nil
	require.Nil(t, cfg.ValidateAndAdjust(sinkURL))
	require.False(t, cfg.Scheduler.EnableTableAcrossNodes)

	// enable the checksum verification, but use blackhole sink
	cfg = GetDefaultReplicaConfig()
	cfg.Integrity.IntegrityCheckLevel = IntegrityCheckLevelCorrectness
	require.NoError(t, cfg.ValidateAndAdjust(sinkURL))
	require.Equal(t, IntegrityCheckLevelNone, cfg.Integrity.IntegrityCheckLevel)
}

func TestIsSinkCompatibleWithSpanReplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		uri        string
		compatible bool
	}{
		{
			name:       "MySQL URI",
			uri:        "mysql://root:111@foo.bar:3306/",
			compatible: false,
		},
		{
			name:       "TiDB URI",
			uri:        "tidb://root:111@foo.bar:3306/",
			compatible: false,
		},
		{
			name:       "MySQL URI",
			uri:        "mysql+ssl://root:111@foo.bar:3306/",
			compatible: false,
		},
		{
			name:       "TiDB URI",
			uri:        "tidb+ssl://root:111@foo.bar:3306/",
			compatible: false,
		},
		{
			name:       "Kafka URI",
			uri:        "kafka://foo.bar:3306/topic",
			compatible: true,
		},
		{
			name:       "Kafka URI",
			uri:        "kafka+ssl://foo.bar:3306/topic",
			compatible: true,
		},
		{
			name:       "Blackhole URI",
			uri:        "blackhole://foo.bar:3306/topic",
			compatible: true,
		},
		{
			name:       "Unknown URI",
			uri:        "unknown://foo.bar:3306",
			compatible: false,
		},
	}

	for _, tt := range tests {
		u, e := url.Parse(tt.uri)
		require.Nil(t, e)
		compatible := isSinkCompatibleWithSpanReplication(u)
		require.Equal(t, compatible, tt.compatible, tt.name)
	}
}
