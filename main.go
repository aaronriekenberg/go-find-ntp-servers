package main

import (
	"cmp"
	"flag"
	"log/slog"
	"maps"
	"net"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beevik/ntp"
)

const (
	ipAddressesPerServerName = 8
)

// flags
var (
	filterDNSToIPV4Only         = flag.Bool("filterDNSToIPV4Only", true, "filter DNS results to IPv4 only")
	maxParallelDNSRequests      = flag.Int("maxParallelDNSRequests", 2, "max parallel DNS requests")
	maxParallelNTPRequests      = flag.Int("maxParallelNTPRequests", 8, "max parallel NTP requests")
	ntpQueryTimeoutMilliseconds = flag.Int64("ntpQueryTimeoutMilliseconds", 1_000, "NTP query timeout milliseconds")
	slogLevel                   slog.Level
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
var (
	dnsQueries             atomic.Int32
	dnsErrors              atomic.Int32
	dnsFilteredResults     atomic.Int32
	dnsUnfilteredResults   atomic.Int32
	foundDuplicateServerIP atomic.Int32
	ntpQueries             atomic.Int32
	ntpErrors              atomic.Int32
)

func serverNames() []string {
	return []string{
		"0.pool.ntp.org",
		"1.pool.ntp.org",
		"2.pool.ntp.org",
		"3.pool.ntp.org",
		"time1.google.com",
		"time2.google.com",
		"time3.google.com",
		"time4.google.com",
		"time1.facebook.com",
		"time2.facebook.com",
		"time3.facebook.com",
		"time4.facebook.com",
		"time5.facebook.com",
		"time.apple.com",
		"time.cloudflare.com",
		"time.aws.com",
		"time.windows.com",
	}
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

func findNTPServers(
	resolvedServerMessageChannel chan<- resolvedServerMessage,
) {
	defer close(resolvedServerMessageChannel)

	querySemaphore := newSemaphore(*maxParallelDNSRequests)

	var wg sync.WaitGroup

	for _, serverName := range serverNames() {
		wg.Add(1)
		go func() {
			querySemaphore.acquire()

			defer wg.Done()
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
				var filteredIP net.IP
				if *filterDNSToIPV4Only {
					filteredIP = ip.To4()
				} else {
					filteredIP = ip
				}
				if filteredIP == nil {
					dnsFilteredResults.Add(1)
				} else {
					dnsUnfilteredResults.Add(1)
					resolvedServerMessageChannel <- resolvedServerMessage{
						ServerName: serverName,
						IPAddr:     filteredIP.String(),
					}
				}
			}
		}()
	}

	wg.Wait()
}

var ipAddrToServerNames map[string]map[string]bool

func isDuplicateServerAddress(
	message resolvedServerMessage,
) bool {
	if ipAddrToServerNames == nil {
		ipAddrToServerNames = make(map[string]map[string]bool)
	}

	ipAddrString := message.IPAddr

	serverNamesForIPAddr := ipAddrToServerNames[ipAddrString]
	if serverNamesForIPAddr == nil {
		serverNamesForIPAddr = make(map[string]bool)
		ipAddrToServerNames[ipAddrString] = serverNamesForIPAddr
	}

	serverNamesForIPAddr[message.ServerName] = true

	duplicateServerNamesForIPAddress := len(serverNamesForIPAddr)
	if duplicateServerNamesForIPAddress > 1 {
		slog.Info("found duplicate server IP address",
			"ipAddress", ipAddrString,
			"duplicateServerNamesForIPAddress", duplicateServerNamesForIPAddress,
			"serverNames", slices.Sorted(maps.Keys(serverNamesForIPAddr)),
		)
		foundDuplicateServerIP.Add(1)
		return true
	}
	return false
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

	querySemaphore := newSemaphore(*maxParallelNTPRequests)

	var readResponsesWG sync.WaitGroup
	readResponsesWG.Add(1)
	go func() {
		for response := range responseChannel {
			responses = append(responses, response)
		}
		readResponsesWG.Done()
	}()

	ntpQueryTimeoutDuration := time.Duration(*ntpQueryTimeoutMilliseconds) * time.Millisecond

	var queryWG sync.WaitGroup
	for message := range resolvedServerMessageChannel {
		if isDuplicateServerAddress(message) {
			continue
		}

		queryWG.Add(1)
		go func() {
			querySemaphore.acquire()

			defer queryWG.Done()
			defer querySemaphore.release()

			slog.Debug("queryNTPServers received message",
				"message", message,
			)

			ntpQueries.Add(1)

			response, err := ntp.QueryWithOptions(
				message.IPAddr,
				ntp.QueryOptions{
					Timeout: ntpQueryTimeoutDuration,
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

		}()
	}

	queryWG.Wait()

	close(responseChannel)

	readResponsesWG.Wait()

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
		slog.Info("ntpServerResponse",
			"serverName", ntpServerResponse.ServerName,
			"ipAddr", ntpServerResponse.IPAddr,
			"stratum", ntpServerResponse.NTPResponse.Stratum,
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

	resolvedServerMessageChannel := make(chan resolvedServerMessage, (*maxParallelDNSRequests)*ipAddressesPerServerName)

	go findNTPServers(resolvedServerMessageChannel)

	ntpServerResponses := queryNTPServers(resolvedServerMessageChannel)

	logResults(ntpServerResponses)
}
