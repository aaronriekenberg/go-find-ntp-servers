package main

import (
	"cmp"
	"flag"
	"log/slog"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/aaronriekenberg/go-find-ntp-servers/internal/finder"
	"github.com/aaronriekenberg/go-find-ntp-servers/internal/querier"
	"github.com/beevik/ntp"
)

// flags
var (
	filterDNSToIPV4Only    = flag.Bool("filterDNSToIPV4Only", true, "filter DNS results to IPv4 only")
	maxParallelDNSRequests = flag.Int("maxParallelDNSRequests", 2, "max parallel DNS requests")
	maxParallelNTPRequests = flag.Int("maxParallelNTPRequests", 8, "max parallel NTP requests")
	ntpQueryTimeout        = flag.Duration("ntpQueryTimeout", 1*time.Second, "NTP query timeout duration")
	queryNTS               = flag.Bool("queryNTS", false, "query servers using NTS")
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

func logResults(
	ntpServerResponses []querier.NTPServerResponse,
) {

	slog.Info("logResults",
		"len(ntpServerResponses)", len(ntpServerResponses),
	)

	slices.SortFunc(
		ntpServerResponses,
		func(a, b querier.NTPServerResponse) int {
			return -cmp.Compare(a.NTPResponse.RootDistance, b.NTPResponse.RootDistance)
		},
	)

	slog.Info("after sort by RootDistance descending")

	for _, resp := range ntpServerResponses {

		rawReferenceID, parsedReferenceID := querier.ParseReferenceID(resp)

		slog.Info("ntpServerResponse",
			"serverName", resp.ServerName,
			"ipAddr", resp.IPAddr,
			"stratum", resp.NTPResponse.Stratum,
			"rawReferenceID", rawReferenceID,
			"parsedReferenceID", parsedReferenceID,
			"clockOffset", resp.NTPResponse.ClockOffset.String(),
			"precision", resp.NTPResponse.Precision.String(),
			"rootDelay", resp.NTPResponse.RootDelay.String(),
			"rootDispersion", resp.NTPResponse.RootDispersion.String(),
			"rtt", resp.NTPResponse.RTT.String(),
			"rootDistance", resp.NTPResponse.RootDistance.String(),
			"usedNTS", resp.UsedNTS,
		)
	}

	slog.Info("metrics",
		"dnsQueries", finder.DNSQueries.Load(),
		"dnsErrors", finder.DNSErrors.Load(),
		"dnsFilteredResults", finder.DNSFilteredResults.Load(),
		"dnsUnfilteredResults", finder.DNSUnfilteredResults.Load(),
		"duplicateServerIPs", querier.DuplicateServerIPs.Load(),
		"duplicateNTSServerNames", querier.DuplicateNTSServerNames.Load(),
		"ntpQueries", querier.NTPQueries.Load(),
		"ntpErrors", querier.NTPErrors.Load(),
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

	resolvedServers := finder.FindNTPServers(
		*queryNTS,
		*filterDNSToIPV4Only,
		*maxParallelDNSRequests,
	)

	responses := querier.QueryNTPServers(
		resolvedServers,
		*queryNTS,
		*maxParallelNTPRequests,
		ntp.QueryOptions{
			Timeout: *ntpQueryTimeout,
		},
	)

	logResults(responses)
}
