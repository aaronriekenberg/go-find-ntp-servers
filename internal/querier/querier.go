package querier

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aaronriekenberg/go-find-ntp-servers/internal/finder"
	"github.com/aaronriekenberg/go-find-ntp-servers/internal/semaphore"
	"github.com/beevik/ntp"
	"github.com/beevik/nts"
)

// Metrics
type atomicMetric = atomic.Uint64

var (
	DuplicateServerIPs      atomicMetric
	DuplicateNTSServerNames atomicMetric
	NTPQueries              atomicMetric
	NTPErrors               atomicMetric
)

// NTPServerResponse holds the result of a single NTP/NTS query.
// Fields are exported to work with slog.
type NTPServerResponse struct {
	ServerName  string
	IPAddr      string
	NTPResponse *ntp.Response
	UsedNTS     bool
}

func duplicateNTPServerCheck() func(finder.ResolvedServer) (duplicate bool) {
	ipAddrToServerNames := make(map[string][]string)

	return func(msg finder.ResolvedServer) (duplicate bool) {
		serverNamesForIPAddr := append(ipAddrToServerNames[msg.IPAddr], msg.ServerName)
		ipAddrToServerNames[msg.IPAddr] = serverNamesForIPAddr

		numServerNamesForIPAddr := len(serverNamesForIPAddr)
		if numServerNamesForIPAddr > 1 {
			slog.Info("found duplicate server IP address",
				"ipAddress", msg.IPAddr,
				"numServerNamesForIPAddr", numServerNamesForIPAddr,
				"serverNames", serverNamesForIPAddr,
			)
			DuplicateServerIPs.Add(1)
			duplicate = true
		}
		return
	}
}

func duplicateNTSServerCheck() func(finder.ResolvedServer) (duplicate bool) {
	seenNTSServerNames := make(map[string]bool)

	return func(msg finder.ResolvedServer) (duplicate bool) {
		if found := seenNTSServerNames[msg.ServerName]; found {
			slog.Info("found duplicate NTS server name",
				"serverName", msg.ServerName,
				"ipAddr", msg.IPAddr,
			)
			DuplicateNTSServerNames.Add(1)
			duplicate = true
		} else {
			seenNTSServerNames[msg.ServerName] = true
		}
		return
	}
}

func duplicateServerCheck(queryNTS bool) func(finder.ResolvedServer) (duplicate bool) {
	if queryNTS {
		return duplicateNTSServerCheck()
	}
	return duplicateNTPServerCheck()
}

// QueryNTPServers concurrently queries all resolved servers and returns the responses.
func QueryNTPServers(
	resolvedServerChannel <-chan finder.ResolvedServer,
	queryNTS bool,
	maxParallelNTPRequests int,
	ntpQueryTimeout time.Duration,
) (responses []NTPServerResponse) {

	slog.Debug("QueryNTPServers starting",
		"queryNTS", queryNTS,
		"maxParallelNTPRequests", maxParallelNTPRequests,
		"ntpQueryTimeout", ntpQueryTimeout.String(),
	)

	responseChannel := make(chan NTPServerResponse, maxParallelNTPRequests)

	var readResponsesWG sync.WaitGroup
	readResponsesWG.Go(func() {
		for response := range responseChannel {
			responses = append(responses, response)
		}
	})

	isDuplicateServer := duplicateServerCheck(queryNTS)

	querySemaphore := semaphore.New(maxParallelNTPRequests)

	var queryWG sync.WaitGroup
	for message := range resolvedServerChannel {
		if isDuplicateServer(message) {
			continue
		}

		querySemaphore.Acquire()
		queryWG.Go(func() {
			defer querySemaphore.Release()

			slog.Debug("QueryNTPServers received message",
				"message", message,
			)

			NTPQueries.Add(1)

			var (
				response *ntp.Response
				err      error
			)

			if queryNTS {
				ntsSession, err := nts.NewSession(message.ServerName)
				if err != nil {
					slog.Error("nts.NewSession error",
						"message", message,
						"err", err,
					)
					NTPErrors.Add(1)
					return
				}
				response, err = ntsSession.QueryWithOptions(&ntp.QueryOptions{
					Timeout: ntpQueryTimeout,
				})
			} else {
				response, err = ntp.QueryWithOptions(
					message.IPAddr,
					ntp.QueryOptions{
						Timeout: ntpQueryTimeout,
					},
				)
			}

			if err != nil {
				slog.Error("NTP query error",
					"message", message,
					"err", err,
					"queryNTS", queryNTS,
				)
				NTPErrors.Add(1)
				return
			}

			slog.Debug("ntp.Query got response",
				"message", message,
				"response", response,
			)

			responseChannel <- NTPServerResponse{
				ServerName:  message.ServerName,
				IPAddr:      message.IPAddr,
				NTPResponse: response,
				UsedNTS:     queryNTS,
			}
		})
	}

	queryWG.Wait()
	close(responseChannel)
	readResponsesWG.Wait()

	return
}

// ParseReferenceID decodes the NTP ReferenceID into raw and human-readable strings.
func ParseReferenceID(resp NTPServerResponse) (rawString string, parsedString string) {
	rawString = fmt.Sprintf("0x%08X", resp.NTPResponse.ReferenceID)

	referenceIDBytes := binary.BigEndian.AppendUint32(nil, resp.NTPResponse.ReferenceID)

	if resp.NTPResponse.Stratum <= 1 {
		parsedString = string(referenceIDBytes)
		return
	}

	if ip := net.IP(referenceIDBytes).To4(); ip != nil {
		parsedString = ip.String()
		return
	}

	return
}
