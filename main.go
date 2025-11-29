package main

import (
	"cmp"
	"encoding/binary"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/beevik/ntp"
)

const (
	ipAddressesPerServerName = 8
)

// flags
var (
	filterDNSToIPV4Only    = flag.Bool("filterDNSToIPV4Only", true, "filter DNS results to IPv4 only")
	maxParallelDNSRequests = flag.Int("maxParallelDNSRequests", 2, "max parallel DNS requests")
	maxParallelNTPRequests = flag.Int("maxParallelNTPRequests", 8, "max parallel NTP requests")
	ntpQueryTimeout        = flag.Duration("ntpQueryTimeout", 1*time.Second, "NTP query timeout duration")
	slogLevel              slog.Level
)

func parseFlags() {
	flag.TextVar(&slogLevel, "slogLevel", slog.LevelInfo, "slog level")

	flag.Parse()
}

func setupSlog() {
	slog.SetDefault(
		slog.New(
			slog.NewJSONHandler(
				os.Stdout,
				&slog.HandlerOptions{
					Level: slogLevel,
				},
			),
		),
	)

	slog.Info("setupSlog",
		"sloglevel", slogLevel,
	)
}

// metrics
type atomicMetric = atomic.Uint64

var (
	dnsQueries             atomicMetric
	dnsErrors              atomicMetric
	dnsFilteredResults     atomicMetric
	dnsUnfilteredResults   atomicMetric
	foundDuplicateServerIP atomicMetric
	ntpQueries             atomicMetric
	ntpErrors              atomicMetric
)

func readServerNames() []string {
	const serversFileName = "servers.toml"

	executablePath, err := os.Executable()
	if err != nil {
		panic(fmt.Errorf("readServerNames: os.Executable error: %w", err))
	}
	executableDirectory := filepath.Dir(executablePath)

	serverFilePath := path.Join(executableDirectory, serversFileName)

	slog.Debug("readServerNames",
		"serverFilePath", serverFilePath,
	)

	var serverConfig struct {
		Servers []string
	}

	_, err = toml.DecodeFile(serverFilePath, &serverConfig)

	if err != nil {
		panic(fmt.Errorf("readServerNames: toml.DecodeFile error: %w", err))
	}

	return serverConfig.Servers
}

type semaphore chan struct{}

func newSemaphore(permits int) semaphore {
	if permits < 1 {
		panic("newSemaphore: permits must be >= 1")
	}
	s := make(chan struct{}, permits)
	for range permits {
		s <- struct{}{}
	}
	return s
}

func (s semaphore) acquire() {
	<-s
}

func (s semaphore) release() {
	s <- struct{}{}
}

// fields are exported to work with slog
type resolvedServerMessage struct {
	ServerName string
	IPAddr     string
}

func findNTPServers() <-chan resolvedServerMessage {
	serverNames := readServerNames()

	resolvedServerMessageChannel := make(
		chan resolvedServerMessage,
		(*maxParallelDNSRequests)*ipAddressesPerServerName,
	)

	go func() {

		defer close(resolvedServerMessageChannel)

		querySemaphore := newSemaphore(*maxParallelDNSRequests)

		var queryWG sync.WaitGroup

		defer queryWG.Wait()

		for _, serverName := range serverNames {

			querySemaphore.acquire()
			queryWG.Go(func() {
				defer querySemaphore.release()

				slog.Debug("resolving server",
					"serverName", serverName,
				)

				dnsQueries.Add(1)
				addrs, err := net.LookupIP(serverName)
				if err != nil {
					slog.Error("net.LookupHost error",
						"serverName", serverName,
						"error", err,
					)
					dnsErrors.Add(1)

					return
				}

				slog.Debug("findNTPServers resolved",
					"server", serverName,
					"addrs", addrs,
				)

				for _, ip := range addrs {
					if *filterDNSToIPV4Only {
						ip = ip.To4()
					}

					if ip == nil {
						dnsFilteredResults.Add(1)
					} else {
						dnsUnfilteredResults.Add(1)
						resolvedServerMessageChannel <- resolvedServerMessage{
							ServerName: serverName,
							IPAddr:     ip.String(),
						}
					}
				}
			})
		}
	}()

	return resolvedServerMessageChannel
}

func duplicateServerAddressCheck() func(resolvedServerMessage) (duplicate bool) {

	ipAddrToServerNames := make(map[string][]string)

	return func(
		message resolvedServerMessage,
	) (duplicate bool) {

		serverNamesForIPAddr := append(ipAddrToServerNames[message.IPAddr], message.ServerName)
		ipAddrToServerNames[message.IPAddr] = serverNamesForIPAddr

		numServerNamesForIPAddr := len(serverNamesForIPAddr)
		if numServerNamesForIPAddr > 1 {
			slog.Info("found duplicate server IP address",
				"ipAddress", message.IPAddr,
				"numServerNamesForIPAddr", numServerNamesForIPAddr,
				"serverNames", serverNamesForIPAddr,
			)
			foundDuplicateServerIP.Add(1)
			duplicate = true
		}
		return
	}
}

// fields are exported to work with slog
type ntpServerResponse struct {
	ServerName  string
	IPAddr      string
	NTPResponse *ntp.Response
}

func queryNTPServers(
	resolvedServerMessageChannel <-chan resolvedServerMessage,
) (responses []ntpServerResponse) {

	responseChannel := make(chan ntpServerResponse, *maxParallelNTPRequests)

	var readResponsesWG sync.WaitGroup
	readResponsesWG.Go(func() {
		for response := range responseChannel {
			responses = append(responses, response)
		}
	})

	isDuplicateServerAddress := duplicateServerAddressCheck()

	querySemaphore := newSemaphore(*maxParallelNTPRequests)

	var queryWG sync.WaitGroup
	for message := range resolvedServerMessageChannel {
		if isDuplicateServerAddress(message) {
			continue
		}

		querySemaphore.acquire()
		queryWG.Go(func() {
			defer querySemaphore.release()

			slog.Debug("queryNTPServers received message",
				"message", message,
			)

			ntpQueries.Add(1)

			response, err := ntp.QueryWithOptions(
				message.IPAddr,
				ntp.QueryOptions{
					Timeout: *ntpQueryTimeout,
				},
			)

			if err != nil {
				slog.Error("ntp.Query error",
					"message", message,
					"err", err,
				)
				ntpErrors.Add(1)

				return
			}

			slog.Debug("ntp.Query got response",
				"message", message,
				"response", response,
			)

			responseChannel <- ntpServerResponse{
				ServerName:  message.ServerName,
				IPAddr:      message.IPAddr,
				NTPResponse: response,
			}

		})
	}

	queryWG.Wait()

	close(responseChannel)

	readResponsesWG.Wait()

	return
}

func parseReferenceID(
	ntpServerResponse ntpServerResponse,
) (rawString string, parsedString string) {

	rawString = fmt.Sprintf("0x%08X", ntpServerResponse.NTPResponse.ReferenceID)

	referenceIDBytes := binary.BigEndian.AppendUint32(nil, ntpServerResponse.NTPResponse.ReferenceID)

	if ntpServerResponse.NTPResponse.Stratum <= 1 {
		parsedString = string(referenceIDBytes)
		return
	}

	if ip := net.IP(referenceIDBytes).To4(); ip != nil {
		parsedString = ip.String()
		return
	}

	return

}

func logResults(
	ntpServerResponses []ntpServerResponse,
) {

	slog.Info("logResults",
		"len(ntpServerResponses)", len(ntpServerResponses),
	)

	slices.SortFunc(
		ntpServerResponses,
		func(a, b ntpServerResponse) int {
			return -cmp.Compare(a.NTPResponse.RootDistance, b.NTPResponse.RootDistance)
		},
	)

	slog.Info("after sort by RootDistance descending")

	for _, ntpServerResponse := range ntpServerResponses {

		rawReferenceID, parsedReferenceID := parseReferenceID(ntpServerResponse)

		slog.Info("ntpServerResponse",
			"serverName", ntpServerResponse.ServerName,
			"ipAddr", ntpServerResponse.IPAddr,
			"stratum", ntpServerResponse.NTPResponse.Stratum,
			"rawReferenceID", rawReferenceID,
			"parsedReferenceID", parsedReferenceID,
			"clockOffset", ntpServerResponse.NTPResponse.ClockOffset.String(),
			"precision", ntpServerResponse.NTPResponse.Precision.String(),
			"rootDelay", ntpServerResponse.NTPResponse.RootDelay.String(),
			"rootDispersion", ntpServerResponse.NTPResponse.RootDispersion.String(),
			"rtt", ntpServerResponse.NTPResponse.RTT.String(),
			"rootDistance", ntpServerResponse.NTPResponse.RootDistance.String(),
		)
	}

	slog.Info("metrics",
		"dnsQueries", dnsQueries.Load(),
		"dnsErrors", dnsErrors.Load(),
		"dnsFilteredResults", dnsFilteredResults.Load(),
		"dnsUnfilteredResults", dnsUnfilteredResults.Load(),
		"foundDuplicateServerIP", foundDuplicateServerIP.Load(),
		"ntpQueries", ntpQueries.Load(),
		"ntpErrors", ntpErrors.Load(),
	)
}

func buildInfoMap() map[string]string {
	buildInfoMap := make(map[string]string)

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		buildInfoMap["GoVersion"] = buildInfo.GoVersion
		for _, setting := range buildInfo.Settings {
			if strings.HasPrefix(setting.Key, "GO") ||
				strings.HasPrefix(setting.Key, "vcs") {
				buildInfoMap[setting.Key] = setting.Value
			}
		}
	}

	return buildInfoMap
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("panic in main",
				"error", err,
			)
			os.Exit(1)
		}
	}()

	parseFlags()

	setupSlog()

	slog.Info("begin main",
		"buildInfoMap", buildInfoMap(),
	)

	resolvedServerMessageChannel := findNTPServers()

	ntpServerResponses := queryNTPServers(resolvedServerMessageChannel)

	logResults(ntpServerResponses)
}
