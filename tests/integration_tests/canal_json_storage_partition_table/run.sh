#!/bin/bash

set -eu

CUR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source $CUR/../_utils/test_prepare
WORK_DIR=$OUT_DIR/$TEST_NAME
CDC_BINARY=cdc.test
SINK_TYPE=$1

function run() {
	if [ "$SINK_TYPE" != "storage" ]; then
		return
	fi

	rm -rf $WORK_DIR && mkdir -p $WORK_DIR
	start_tidb_cluster --workdir $WORK_DIR
	cd $WORK_DIR

	run_sql "set @@global.tidb_enable_exchange_partition=on" ${DOWN_TIDB_HOST} ${DOWN_TIDB_PORT}
	# TODO(CharlesCheung): remove this after schema level ddl is supported by storage sink
	run_sql "create database partition_table;" ${DOWN_TIDB_HOST} ${DOWN_TIDB_PORT}

	run_cdc_server --workdir $WORK_DIR --binary $CDC_BINARY --addr "127.0.0.1:8300" --logsuffix cdc0
	run_cdc_server --workdir $WORK_DIR --binary $CDC_BINARY --addr "127.0.0.1:8301" --logsuffix cdc1
	run_cdc_server --workdir $WORK_DIR --binary $CDC_BINARY --addr "127.0.0.1:8302" --logsuffix cdc2

	SINK_URI="file://$WORK_DIR/storage_test?flush-interval=5s&enable-tidb-extension=true"
	run_cdc_cli changefeed create --sink-uri="$SINK_URI" --config=$CUR/conf/changefeed.toml

	run_sql_file $CUR/data/prepare.sql ${UP_TIDB_HOST} ${UP_TIDB_PORT}

	run_storage_consumer $WORK_DIR $SINK_URI $CUR/conf/changefeed.toml ""
	sleep 8

	# sync_diff can't check non-exist table, so we check expected tables are created in downstream first
	check_table_exists partition_table.t ${DOWN_TIDB_HOST} ${DOWN_TIDB_PORT}
	check_table_exists partition_table.t1 ${DOWN_TIDB_HOST} ${DOWN_TIDB_PORT}
	# check_table_exists partition_table.t2 ${DOWN_TIDB_HOST} ${DOWN_TIDB_PORT}
	check_table_exists partition_table.finish_mark ${DOWN_TIDB_HOST} ${DOWN_TIDB_PORT}
	check_sync_diff $WORK_DIR $CUR/conf/diff_config.toml

	cleanup_process $CDC_BINARY
}

trap stop_tidb_cluster EXIT
run $*
check_logs $WORK_DIR
echo "[$(date)] <<<<<< run test case $TEST_NAME success! >>>>>>"
