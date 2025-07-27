package main

import (
	"cmp"
	"flag"
	"log/slog"
	"net"
	"os"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/beevik/ntp"
)

// flags
var (
	filterDNSToIPV4Only    = flag.Bool("filterDNSToIPV4Only", true, "filter DNS results to IPv4 only")
	maxParallelDNSRequests = flag.Int("maxParallelDNSRequests", 8, "max parallel DNS requests")
	maxParallelNTPRequests = flag.Int("maxParallelNTPRequests", 8, "max parallel NTP requests")
	sloglevel              slog.Level
)

func parseFlags() {
	flag.TextVar(&sloglevel, "slogLevel", slog.LevelInfo, "slog level")

	flag.Parse()
}

func setupSlog() {
	slog.SetDefault(
		slog.New(
			slog.NewJSONHandler(
				os.Stdout,
				&slog.HandlerOptions{
					Level: sloglevel,
				},
			),
		),
	)

	slog.Info("setupSlog",
		"sloglevel", sloglevel,
	)
}

// metrics
var (
	dnsQueries     atomic.Int32
	dnsErrors      atomic.Int32
	ntpQueries     atomic.Int32
	ntpQueryErrors atomic.Int32
)

type semaphore chan struct{}

func newSemaphore(permits int) semaphore {
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
	IPAddr     net.IP
}

func findNTPServers(
	resolvedServerMessageChannel chan resolvedServerMessage,
) {
	defer close(resolvedServerMessageChannel)

	serverNames := []string{
		"0.pool.ntp.org",
		"1.pool.ntp.org",
		"2.pool.ntp.org",
		"3.pool.ntp.org",
		"time.google.com",
		"time.facebook.com",
		"time3.facebook.com",
		"time.apple.com",
		"time.cloudflare.com",
		"time.aws.com",
		"time.windows.com",
	}

	permits := newSemaphore(*maxParallelDNSRequests)

	var wg sync.WaitGroup

	for _, serverName := range serverNames {
		wg.Add(1)
		go func() {
			permits.acquire()

			defer wg.Done()
			defer permits.release()

			slog.Info("resolving server",
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

			slog.Info("findNTPServers resolved",
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
				if filteredIP != nil {
					resolvedServerMessageChannel <- resolvedServerMessage{
						ServerName: serverName,
						IPAddr:     filteredIP,
					}
				}
			}
		}()
	}

	wg.Wait()
}

// fields are exported to work with slog
type ntpServerResponse struct {
	ServerName  string
	IPAddr      string
	NTPResponse *ntp.Response
}

func queryNTPServers(
	resolvedServerMessageChannel chan resolvedServerMessage,
) (responses []ntpServerResponse) {

	responseChannel := make(chan ntpServerResponse, *maxParallelNTPRequests)

	queryPermits := newSemaphore(*maxParallelNTPRequests)

	var readResponsesWG sync.WaitGroup
	readResponsesWG.Add(1)
	go func() {
		for response := range responseChannel {
			responses = append(responses, response)
		}
		readResponsesWG.Done()
	}()

	var queryWG sync.WaitGroup
	for message := range resolvedServerMessageChannel {
		queryWG.Add(1)
		go func() {
			queryPermits.acquire()

			defer queryPermits.release()
			defer queryWG.Done()

			slog.Info("queryNTPServers received",
				"message", message,
			)

			ntpQueries.Add(1)

			response, err := ntp.Query(
				message.IPAddr.String(),
			)

			if err != nil {
				slog.Error("ntp.Query error",
					"message", message,
					"err", err,
				)
				ntpQueryErrors.Add(1)

				return
			}

			slog.Info("ntp.Query got response",
				"response", response,
			)

			responseChannel <- ntpServerResponse{
				ServerName:  message.ServerName,
				IPAddr:      message.IPAddr.String(),
				NTPResponse: response,
			}

		}()
	}

	queryWG.Wait()

	close(responseChannel)

	readResponsesWG.Wait()

	return
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

	resolvedServerMessageChannel := make(chan resolvedServerMessage, *maxParallelDNSRequests)

	go findNTPServers(resolvedServerMessageChannel)

	ntpServerResponses := queryNTPServers(resolvedServerMessageChannel)

	slog.Info("after queryNTPServers",
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
		"ntpQueries", ntpQueries.Load(),
		"ntpQueryErrors", ntpQueryErrors.Load(),
	)
}
