# WetLog
![test and build](https://github.com/kenjords/wetlog/.github/workflows/test_and_build.yml/badge.svg) 
![release](https://github.com/kenjords/wetlog/.github/workflows/release_build.yml/badge.svg)

WetLog is a simple log file viewer for DS Diagnistics packaged system.log files.  
It was developed as an artifact from prototyping for a larger project around DS case investigation tooling.  
I created it as its own application as I saw that it could potentially be useful as it is.
It is not intended to be a full featured log viewer, but rather a simple tool to help with the investigation of DS cases.
It allows being able to view and segregate logs by DC. It also has some additional sorting features (thanks og-ken)  
for the idea. 

The name WetLog is a reference to wading through the swamp of logs that is the system.log file to find only those logs that are   
relevant to the case at hand.

--- 

## Installation and Usage

### Installation

WetLog is a single executable file. It can be downloaded from the [releases]() page. 


### Usage

WetLog is a command-line application and has a fairly simple command syntax. 

#### Command syntax

```bash
./wetlog -file <path to nodetool/status file> [-list-dcs]|[-datacenters <dcname1,dcname2>] [-query <"query term 1", "query term 2">] [-sort <sort criteria> ] path_to_diagnostics_package   
```

### Examples

List dc's in the diagnostics package
```bash
./wetlog -file /path/to/diagnostics/package/nodetool/status -list-dcs ./
---
DC1
DC2
DC3
```
Get logs from a single Datacenter
```bash
./wetlog -file /path/to/diagnostics/package/nodetool/status -datacenters DC1  prod_dc_information/
```
Get logs from multiple Datacenters sorted by Log level
```bash
./wetlog -file /path/to/diagnostics/package/nodetool/status -datacenters DC1,DC2 -sort LogLevel prod_dc_information/
```
Get logs from multiple Datacenters sorted by Log level and only show logs that contain the term "ERROR" and "client"
```bash
./wetlog -file /path/to/diagnostics/package/nodetool/status -datacenters DC1,DC2 -sort LogLevel -query "ERROR","client" prod_dc_information/
```

#### Flags

| Flag      | Description                                                                              |
|-----------|------------------------------------------------------------------------------------------|
| -help     | Prints help information                                                                  |
| -version  | Prints version information                                                               |
| -file | This a mandatory flag that specifies the path to an instance of the nodetool status file |
| -list-dcs | This flag will print out a list of the available DCs reperesented in the Diags packageg  |
| -datacenters | This flag is mandatory for searches, but multiple DCs can be specified.                  |
| -query | A comma delimited list of queries that are parsed sequentially.  |
| -sort | This flag will sort the output by specified criteria. |

#### Sort Criteria

| Criteria  | Description                                                    |
|-----------|----------------------------------------------------------------|
| date      | Sorts the output by timestamp. This is the default behavior    |
| loglevel  | Sorts the output by log level.                                 |
| linenumer | Sorts the output by line number.                               |
| nodeip | Sorts the output by node ip.                                   |

### Querying data

Currently, the behavior of the query flag is to parse the comma delimited list of queries sequentially.
This means that it will match the first term, then match with logs returned from the first term that contain the second  
term. 
This may change in the future depending on proves the most useful in practice. 
