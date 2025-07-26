package main

import (
	"cmp"
	"flag"
	"log/slog"
	"net"
	"os"
	"slices"

	"github.com/beevik/ntp"
)

var (
	network   = flag.String("network", "udp", "network to use")
	sloglevel slog.Level
)

func parseFlags() {
	flag.TextVar(&sloglevel, "sloglevel", slog.LevelInfo, "slog level")

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

type resolvedServerMessage struct {
	serverName string
	ipAddr     *net.IPAddr
}

func findNTPServers(
	resolvedServerMessageChannel chan resolvedServerMessage,
) {
	defer close(resolvedServerMessageChannel)

	const network = "ip4"

	serverNames := []string{
		"0.pool.ntp.org",
		"1.pool.ntp.org",
		"2.pool.ntp.org",
		"3.pool.ntp.org",
		"time.google.com",
		"time3.facebook.com",
		"time.apple.com",
		"time.cloudflare.com",
	}

	for _, serverName := range serverNames {
		slog.Info("resolving server",
			"serverName", serverName,
			"network", network,
		)

		ipAddr, err := net.ResolveIPAddr(network, serverName)
		if err != nil {
			slog.Error("net.ResolveIPAddr error",
				"serverName", serverName,
				"error", err,
			)
			continue
		}

		slog.Info("findNTPServers resolved",
			"server", serverName,
			"ipAddr", ipAddr,
		)

		resolvedServerMessageChannel <- resolvedServerMessage{
			serverName: serverName,
			ipAddr:     ipAddr,
		}
	}
}

type ntpServerResponse struct {
	serverName  string
	ipAddr      *net.IPAddr
	ntpResponse *ntp.Response
}

func queryNTPServers(
	resolvedServerMessageChannel chan resolvedServerMessage,
) []ntpServerResponse {

	var responses []ntpServerResponse

	for message := range resolvedServerMessageChannel {
		slog.Info("queryNTPServers received",
			"message", message,
		)

		response, err := ntp.Query(
			message.ipAddr.IP.String(),
		)

		if err != nil {
			slog.Error("ntp.Query error",
				"message", message,
				"err", err,
			)
		}

		slog.Info("ntp.Query got response",
			"response", response,
		)

		responses = append(responses, ntpServerResponse{
			serverName:  message.serverName,
			ipAddr:      message.ipAddr,
			ntpResponse: response,
		})
	}

	return responses
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

	resolvedServerMessageChannel := make(chan resolvedServerMessage, 4)

	go findNTPServers(resolvedServerMessageChannel)

	ntpServerResponses := queryNTPServers(resolvedServerMessageChannel)

	slog.Info("after queryNTPServers",
		"len(ntpServerResponses)", len(ntpServerResponses),
	)

	slices.SortFunc(
		ntpServerResponses,
		func(a, b ntpServerResponse) int {
			return -cmp.Compare(a.ntpResponse.RTT, b.ntpResponse.RTT)
		},
	)

	slog.Info("after sort by RTT descending")

	for _, ntpServerResponse := range ntpServerResponses {
		slog.Info("ntpServerResponse",
			"serverName", ntpServerResponse.serverName,
			"ipAddr", ntpServerResponse.ipAddr,
			"clockOffset", ntpServerResponse.ntpResponse.ClockOffset.String(),
			"precision", ntpServerResponse.ntpResponse.Precision.String(),
			"rootDelay", ntpServerResponse.ntpResponse.RootDelay.String(),
			"rootDispersion", ntpServerResponse.ntpResponse.RootDispersion.String(),
			"rootDistance", ntpServerResponse.ntpResponse.RootDistance.String(),
			"rtt", ntpServerResponse.ntpResponse.RTT.String(),
		)
	}

}
