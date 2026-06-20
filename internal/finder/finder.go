package finder

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/BurntSushi/toml"
)

const (
	ipAddressesPerServerName = 8
)

// Metrics
type atomicMetric = atomic.Uint64

var (
	DNSQueries           atomicMetric
	DNSErrors            atomicMetric
	DNSFilteredResults   atomicMetric
	DNSUnfilteredResults atomicMetric
)

// ResolvedServer holds a server name and its resolved IP address.
// Fields are exported to work with slog.
type ResolvedServer struct {
	ServerName string
	IPAddr     string
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

func ReadServerNames(queryNTS bool) []string {
	const ntpServersFileName = "ntp-servers.toml"
	const ntsServersFileName = "nts-servers.toml"

	executablePath, err := os.Executable()
	if err != nil {
		panic(fmt.Errorf("ReadServerNames: os.Executable error: %w", err))
	}

	var serversFileName string
	if queryNTS {
		serversFileName = ntsServersFileName
	} else {
		serversFileName = ntpServersFileName
	}
	serverFilePath := path.Join(filepath.Dir(executablePath), serversFileName)

	slog.Debug("ReadServerNames",
		"serverFilePath", serverFilePath,
	)

	var serverConfig struct {
		Servers []string
	}

	_, err = toml.DecodeFile(serverFilePath, &serverConfig)
	if err != nil {
		panic(fmt.Errorf("ReadServerNames: toml.DecodeFile error: %w", err))
	}

	return serverConfig.Servers
}

// FindNTPServers resolves server hostnames to IP addresses via DNS and returns
// a channel of ResolvedServer messages. In NTS mode, DNS resolution is skipped.
func FindNTPServers(
	queryNTS bool,
	filterDNSToIPV4Only bool,
	maxParallelDNSRequests int,
) <-chan ResolvedServer {
	serverNames := ReadServerNames(queryNTS)

	if queryNTS {
		slog.Debug("querying NTS servers, skipping DNS resolution",
			"numServerNames", len(serverNames),
		)
		resolvedServerChannel := make(chan ResolvedServer, len(serverNames))
		for _, serverName := range serverNames {
			resolvedServerChannel <- ResolvedServer{
				ServerName: serverName,
				IPAddr:     "",
			}
		}
		close(resolvedServerChannel)
		return resolvedServerChannel
	}

	resolvedServerChannel := make(
		chan ResolvedServer,
		maxParallelDNSRequests*ipAddressesPerServerName,
	)
	go func() {
		defer close(resolvedServerChannel)

		querySemaphore := newSemaphore(maxParallelDNSRequests)

		var queryWG sync.WaitGroup
		defer queryWG.Wait()

		for _, serverName := range serverNames {
			querySemaphore.acquire()
			queryWG.Go(func() {
				defer querySemaphore.release()

				slog.Debug("resolving server",
					"serverName", serverName,
				)

				DNSQueries.Add(1)
				addrs, err := net.LookupIP(serverName)
				if err != nil {
					slog.Error("net.LookupHost error",
						"serverName", serverName,
						"error", err,
					)
					DNSErrors.Add(1)
					return
				}

				slog.Debug("FindNTPServers resolved",
					"server", serverName,
					"addrs", addrs,
				)

				for _, ip := range addrs {
					if filterDNSToIPV4Only {
						ip = ip.To4()
					}

					if ip == nil {
						DNSFilteredResults.Add(1)
					} else {
						DNSUnfilteredResults.Add(1)
						resolvedServerChannel <- ResolvedServer{
							ServerName: serverName,
							IPAddr:     ip.String(),
						}
					}
				}
			})
		}
	}()

	return resolvedServerChannel
}
