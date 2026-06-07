# go-find-ntp-servers

Given a list of [NTP servers](ntp-servers.toml) or [NTS servers](nts-servers.toml), find the server having the lowest [Root Distance](https://datatracker.ietf.org/doc/html/rfc5905#appendix-A.5.5.2)

Use `-queryNTS=true` to query servers with Network Time Security (NTS). The default is `false`.

Example:
```
$ go-find-ntp-servers  -queryNTS=true | tail -2 | jq
{
  "time": "2026-06-07T13:09:07.452646587-05:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "ntp1.wiktel.com",
  "ipAddr": "69.89.207.99",
  "stratum": 1,
  "rawReferenceID": "0x50505300",
  "parsedReferenceID": "PPS\u0000",
  "clockOffset": "1.404497ms",
  "precision": "119ns",
  "rootDelay": "0s",
  "rootDispersion": "1.052856ms",
  "rtt": "24.021352ms",
  "rootDistance": "13.063532ms",
  "usedNTS": true
}
{
  "time": "2026-06-07T13:09:07.452681253-05:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 8,
  "dnsErrors": 0,
  "dnsFilteredResults": 9,
  "dnsUnfilteredResults": 9,
  "duplicateServerIPs": 0,
  "duplicateNTSServerNames": 1,
  "ntpQueries": 8,
  "ntpErrors": 0
}
```
