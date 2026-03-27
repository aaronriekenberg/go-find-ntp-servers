# go-find-ntp-servers

Given a [list of NTP server names](servers.toml), find the server having the lowest [Root Distance](https://datatracker.ietf.org/doc/html/rfc5905#appendix-A.5.5.2)

Example:
```
$ go-find-ntp-servers | tail -2 | jq

{
  "time": "2026-03-27T05:38:19.553329923-05:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "time3.facebook.com",
  "ipAddr": "129.134.25.123",
  "stratum": 1,
  "rawReferenceID": "0x4D535031",
  "parsedReferenceID": "MSP1",
  "clockOffset": "303.003µs",
  "precision": "0s",
  "rootDelay": "0s",
  "rootDispersion": "15.259µs",
  "rtt": "9.810333ms",
  "rootDistance": "4.920425ms"
}
{
  "time": "2026-03-27T05:38:19.553360765-05:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 19,
  "dnsErrors": 0,
  "dnsFilteredResults": 27,
  "dnsUnfilteredResults": 41,
  "duplicateServerIPs": 0,
  "ntpQueries": 41,
  "ntpErrors": 0
}
```
