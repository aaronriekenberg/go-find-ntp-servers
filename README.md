# go-find-ntp-servers

Given a list of [NTP servers](ntp-servers.toml) or [NTS servers](nts-servers.toml), find the server having the lowest [Root Distance](https://datatracker.ietf.org/doc/html/rfc5905#appendix-A.5.5.2)

Use `-queryNTS=true` to query servers with Network Time Security (NTS). The default is `false`.

Examples:
```
go-find-ntp-servers | tail -2 | jq
{
  "time": "2026-09-24T05:25:11.850695713-05:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "time3.facebook.com",
  "ipAddr": "129.134.25.123",
  "stratum": 1,
  "rawReferenceID": "0x4D535031",
  "parsedReferenceID": "MSP1",
  "clockOffset": "1.320508ms",
  "precision": "0s",
  "rootDelay": "0s",
  "rootDispersion": "15.259µs",
  "rtt": "7.376846ms",
  "rootDistance": "3.703682ms",
  "usedNTS": false
}
{
  "time": "2026-09-24T05:25:11.850715764-05:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 19,
  "dnsErrors": 0,
  "dnsFilteredResults": 27,
  "dnsUnfilteredResults": 41,
  "duplicateServerIPs": 1,
  "duplicateNTSServerNames": 0,
  "ntpQueries": 40,
  "ntpErrors": 0
}

$ go-find-ntp-servers  -queryNTS       | tail -2 | jq
{
  "time": "2026-09-24T05:25:29.385275604-05:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "time2.mbix.ca",
  "ipAddr": "",
  "stratum": 1,
  "rawReferenceID": "0x50505300",
  "parsedReferenceID": "PPS",
  "clockOffset": "190.701µs",
  "precision": "119ns",
  "rootDelay": "0s",
  "rootDispersion": "1.037598ms",
  "rtt": "13.326302ms",
  "rootDistance": "7.700749ms",
  "usedNTS": true
}
{
  "time": "2026-09-24T05:25:29.385285304-05:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 0,
  "dnsErrors": 0,
  "dnsFilteredResults": 0,
  "dnsUnfilteredResults": 0,
  "duplicateServerIPs": 0,
  "duplicateNTSServerNames": 0,
  "ntpQueries": 20,
  "ntpErrors": 1
}
```
