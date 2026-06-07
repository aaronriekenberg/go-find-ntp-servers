# go-find-ntp-servers

Given a list of [NTP servers](ntp-servers.toml) or [NTS servers](nts-servers.toml), find the server having the lowest [Root Distance](https://datatracker.ietf.org/doc/html/rfc5905#appendix-A.5.5.2)

Use `-queryNTS=true` to query servers with Network Time Security (NTS). The default is `false`.

Examples:
```
$ ./go-find-ntp-servers | tail -2 | jq
{
  "time": "2026-06-07T15:27:36.584614441-05:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "time3.facebook.com",
  "ipAddr": "129.134.25.123",
  "stratum": 1,
  "rawReferenceID": "0x4D535031",
  "parsedReferenceID": "MSP1",
  "clockOffset": "-1.292511ms",
  "precision": "0s",
  "rootDelay": "0s",
  "rootDispersion": "15.259µs",
  "rtt": "9.908813ms",
  "rootDistance": "4.969665ms",
  "usedNTS": false
}
{
  "time": "2026-06-07T15:27:36.584626122-05:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 19,
  "dnsErrors": 0,
  "dnsFilteredResults": 26,
  "dnsUnfilteredResults": 41,
  "duplicateServerIPs": 0,
  "duplicateNTSServerNames": 0,
  "ntpQueries": 41,
  "ntpErrors": 0
}

$ go-find-ntp-servers -queryNTS=true | tail -2 | jq
{
  "time": "2026-06-07T15:26:05.150722498-05:00",
  "level": "INFO",
  "msg": "ntpServerResponse",
  "serverName": "ntp2.wiktel.com",
  "ipAddr": "",
  "stratum": 1,
  "rawReferenceID": "0x47505300",
  "parsedReferenceID": "GPS\u0000",
  "clockOffset": "1.577024ms",
  "precision": "119ns",
  "rootDelay": "0s",
  "rootDispersion": "1.00708ms",
  "rtt": "24.802577ms",
  "rootDistance": "13.408368ms",
  "usedNTS": true
}
{
  "time": "2026-06-07T15:26:05.150740059-05:00",
  "level": "INFO",
  "msg": "metrics",
  "dnsQueries": 0,
  "dnsErrors": 0,
  "dnsFilteredResults": 0,
  "dnsUnfilteredResults": 0,
  "duplicateServerIPs": 0,
  "duplicateNTSServerNames": 0,
  "ntpQueries": 8,
  "ntpErrors": 0
}
```
