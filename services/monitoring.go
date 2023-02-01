package services

import (
	"context"
	"eth2-exporter/db"
	"eth2-exporter/utils"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

func startMonitoringService(wg *sync.WaitGroup) {
	defer wg.Done()

	go startClDataMonitoringService()
	go startElDataMonitoringService()
	go startRedisMonitoringService()
	go startApiMonitoringService()
	go startAppMonitoringService()
	go startServicesMonitoringService()
}

// The cl data monitoring service will check that the data in the validators, blocks & epochs tables is up to date
func startClDataMonitoringService() {

	name := "monitoring_cl_data"
	firstRun := true
	for {
		if !firstRun {
			time.Sleep(time.Minute)
		}
		firstRun = false

		// retrieve the max attestationslot from the validators table and check that it is not older than 15 minutes
		var maxAttestationSlot uint64
		err := db.WriterDb.Get(&maxAttestationSlot, "SELECT MAX(lastattestationslot) FROM validators;")
		if err != nil {
			logger.Errorf("error retrieving max attestation slot from validators table: %w", err)
			continue
		}

		if time.Since(utils.SlotToTime(maxAttestationSlot)) > time.Minute*15 {
			errorMsg := fmt.Errorf("error: max attestation slot is older than 15 minutes: %v", time.Since(utils.SlotToTime(maxAttestationSlot)))
			logger.Error(errorMsg)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		// retrieve the max slot from the blocks table and check tat it is not older than 15 minutes
		var maxSlot uint64
		err = db.WriterDb.Get(&maxSlot, "SELECT MAX(slot) FROM blocks;")
		if err != nil {
			logger.Errorf("error retrieving max slot from blocks table: %w", err)
			continue
		}

		if time.Since(utils.SlotToTime(maxSlot)) > time.Minute*15 {
			errorMsg := fmt.Errorf("error: max slot in blocks table is older than 15 minutes: %v", time.Since(utils.SlotToTime(maxAttestationSlot)))
			logger.Error(errorMsg)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		// retrieve the max epoch from the epochs table and check tat it is not older than 15 minutes
		var maxEpoch uint64
		err = db.WriterDb.Get(&maxEpoch, "SELECT MAX(epoch) FROM epochs;")
		if err != nil {
			logger.Errorf("error retrieving max slot from blocks table: %w", err)
			continue
		}

		if time.Since(utils.EpochToTime(maxEpoch)) > time.Minute*15 {
			errorMsg := fmt.Errorf("error: max epoch in epochs table is older than 15 minutes: %v", time.Since(utils.SlotToTime(maxAttestationSlot)))
			logger.Error(errorMsg)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		ReportStatus(name, "OK", nil)
	}
}

func startElDataMonitoringService() {

	name := "monitoring_el_data"
	firstRun := true
	for {
		if !firstRun {
			time.Sleep(time.Minute)
		}
		firstRun = false

		// check latest eth1 indexed block
		numberBlocksTable, err := db.BigtableClient.GetLastBlockInBlocksTable()
		if err != nil {
			errorMsg := fmt.Errorf("error: could not retrieve latest block number from the blocks table: %v", err)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}
		blockBlocksTable, err := db.BigtableClient.GetBlockFromBlocksTable(uint64(numberBlocksTable))
		if err != nil {
			errorMsg := fmt.Errorf("error: could not retrieve latest block from the blocks table: %v", err)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}
		if blockBlocksTable.Time.AsTime().Before(time.Now().Add(time.Minute * -13)) {
			errorMsg := fmt.Errorf("error: last block in blocks table is more than 13 minutes old (check eth1 indexer)")
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		// check if eth1 indices are up to date
		numberDataTable, err := db.BigtableClient.GetLastBlockInDataTable()
		if err != nil {
			errorMsg := fmt.Errorf("error: could not retrieve latest block number from the data table: %v", err)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		if numberDataTable < numberBlocksTable-32 {
			errorMsg := fmt.Errorf("error: data table is lagging behind the blocks table (check eth1 indexer)")
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}
		ReportStatus(name, "OK", nil)
	}
}

func startRedisMonitoringService() {

	name := "monitoring_redis"
	firstRun := true
	for {
		if !firstRun {
			time.Sleep(time.Minute)
		}
		firstRun = false

		rdc := redis.NewClient(&redis.Options{
			Addr: utils.Config.RedisCacheEndpoint,
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		if err := rdc.Ping(ctx).Err(); err != nil {
			cancel()
			ReportStatus(name, err.Error(), nil)
			rdc.Close()
			continue
		}
		cancel()
		rdc.Close()
		ReportStatus(name, "OK", nil)
	}
}

func startApiMonitoringService() {

	name := "monitoring_api"
	firstRun := true

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	for {
		if !firstRun {
			time.Sleep(time.Minute)
		}
		firstRun = false

		url := "https://" + utils.Config.Frontend.SiteDomain + "/api/v1/epoch/latest"
		resp, err := client.Get(url)

		if err != nil {
			logger.Error(err)
			ReportStatus(name, err.Error(), nil)
			continue
		}

		if resp.StatusCode != 200 {
			errorMsg := fmt.Errorf("error: api epoch / latest endpoint returned a non 200 status: %v", resp.StatusCode)
			logger.Error(errorMsg)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		ReportStatus(name, "OK", nil)
	}
}

func startAppMonitoringService() {

	name := "monitoring_app"
	firstRun := true

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	for {
		if !firstRun {
			time.Sleep(time.Minute)
		}
		firstRun = false

		url := "https://" + utils.Config.Frontend.SiteDomain + "/api/v1/app/dashboard"
		resp, err := client.Post(url, "application/json", strings.NewReader(`{"indicesOrPubkey": "1,2"}`))

		if err != nil {
			logger.Error(err)
			ReportStatus(name, err.Error(), nil)
			continue
		}

		if resp.StatusCode != 200 {
			errorMsg := fmt.Errorf("error: api app endpoint returned a non 200 status: %v", resp.StatusCode)
			logger.Error(errorMsg)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		ReportStatus(name, "OK", nil)
	}
}

func startServicesMonitoringService() {

	name := "monitoring_services"
	firstRun := true

	for {
		if !firstRun {
			time.Sleep(time.Minute)
		}
		firstRun = false

		servicesToCheck := []string{
			"eth1indexer",
			"slotVizUpdater",
			"slotUpdater",
			"latestProposedSlotUpdater",
			"epochUpdater",
			"rewardsExporter",
			"mempoolUpdater",
			"indexPageDataUpdater",
			"latestBlockUpdater",
			"notification-collector",
			//"notification-sender", //exclude for now as the sender is only running on mainnet
			"relaysUpdater",
			"ethstoreExporter",
			"statsUpdater",
			"poolsUpdater",
			"epochExporter",
			"statistics",
			"poolInfoUpdater",
			"epochExporter",
		}

		type serviceStatus struct {
			Name   string
			Status string
		}

		var res []*serviceStatus

		err := db.WriterDb.Select(&res, `select name, status from service_status where last_update > now() - interval '15 minutes' order by last_update desc;`)

		if err != nil {
			errorMsg := fmt.Errorf("error: could not retrieve service status from the service_status table: %v", err)
			ReportStatus(name, errorMsg.Error(), nil)
			continue
		}

		statusMap := make(map[string]string)

		for _, s := range res {
			_, exists := statusMap[s.Name]

			if !exists {
				statusMap[s.Name] = s.Status
			}
		}

		hasError := false
		for _, serviceName := range servicesToCheck {
			if statusMap[serviceName] != "Running" {
				errorMsg := fmt.Errorf("error: service %v has unexpected state %v", serviceName, statusMap[serviceName])
				ReportStatus(name, errorMsg.Error(), nil)
				hasError = true
				break
			}
		}

		if !hasError {
			ReportStatus(name, "OK", nil)
		}

		_, err = db.WriterDb.Exec("DELETE FROM service_status WHERE last_update < NOW() - INTERVAL '1 WEEK'")

		if err != nil {
			logger.Errorf("error cleaning up service_status table")
		}
	}
}
