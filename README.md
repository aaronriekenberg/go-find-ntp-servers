# go-find-ntp-servers

Given a [list of NTP server names](servers.toml), find the server having the lowest [Root Distance](https://datatracker.ietf.org/doc/html/rfc5905#appendix-A.5.5.2)

Example:
```
$ go-find-ntp-servers | tail -2 | jq

{
  "time": "2025-12-04T05:37:06.048627637-06:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "time3.facebook.com",
  "ipAddr": "129.134.25.123",
  "stratum": 1,
  "rawReferenceID": "0x4D535031",
  "parsedReferenceID": "MSP1",
  "clockOffset": "11.992814ms",
  "precision": "0s",
  "rootDelay": "0s",
  "rootDispersion": "15.259µs",
  "rtt": "5.71137ms",
  "rootDistance": "2.870944ms"
}
{
  "time": "2025-12-04T05:37:06.048657383-06:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 19,
  "dnsErrors": 0,
  "dnsFilteredResults": 27,
  "dnsUnfilteredResults": 41,
  "foundDuplicateServerIP": 1,
  "ntpQueries": 40,
  "ntpErrors": 1
}
```
