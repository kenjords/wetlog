package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Node represents a node in the cluster.
type Node struct {
	Address    string
	Datacenter string
}

// LogLevel represents a log level as an iota integer constant. The iota starts at 0 and increments by 1 for each LogLevel higher.
type LogLevel int

const (
	// DEBUG is the lowest log level in Cassandra and will have detailed information about Cassandra actions.
	DEBUG LogLevel = iota // 0
	// INFO is the second lowest log level and is typically used in Cassandra for informative actions.
	INFO // 1
	// WARN in Cassandra is used to indicate that something is not right, but Cassandra can still function.
	WARN // 2
	// ERROR is the highest log level in Cassandra and is used to indicate that the particular action taken by cassandra has failed.
	ERROR //	3
)

// LogEntry represents a log entry.
type LogEntry struct {
	LogLevel   LogLevel  // LogLevel is the log level of the entry.
	Date       time.Time // Date is the date of the entry.
	LineNumber int       // LineNumber is the line number of the entry.
	NodeIP     string    // NodeIP is the IP address of the node that generated the entry.
	FilePath   string    // FilePath is the path to the log file that generated the entry.
	Message    string    // Message is the message of the entry.
}

// LogEntries is a pointer to a slice of LogEntry.
type LogEntries []*LogEntry

// Len returns the length of the LogEntries slice.
func (s LogEntries) Len() int { return len(s) }

// Swap swaps the elements at the given indices.
func (s LogEntries) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// ByDate sorts LogEntries by date.
type ByDate struct{ LogEntries }

// ByLogLevel sorts LogEntries by log level.
type ByLogLevel struct{ LogEntries }

// ByLineNumber sorts LogEntries by line number.
type ByLineNumber struct{ LogEntries }

// ByNodeIP sorts LogEntries by node IP.
type ByNodeIP struct{ LogEntries }

// Less returns true if the date of the LogEntry at index i is before the date of the LogEntry at index j.
func (s ByDate) Less(i, j int) bool { return s.LogEntries[i].Date.Before(s.LogEntries[j].Date) }

// Less returns true if the log level of the LogEntry at index i is before the log level of the LogEntry at index j.
func (s ByLogLevel) Less(i, j int) bool { return s.LogEntries[i].LogLevel < s.LogEntries[j].LogLevel }

// Less returns true if the line number of the LogEntry at index i is before the line number of the LogEntry at index j.
func (s ByLineNumber) Less(i, j int) bool {
	return s.LogEntries[i].LineNumber < s.LogEntries[j].LineNumber
}

// Less returns true if the node IP of the LogEntry at index i is before the node IP of the LogEntry at index j.
func (s ByNodeIP) Less(i, j int) bool {
	ip1 := net.ParseIP(s.LogEntries[i].NodeIP)
	ip2 := net.ParseIP(s.LogEntries[j].NodeIP)

	// Fallback to lexicographical comparison if parsing fails
	if ip1 == nil || ip2 == nil {
		return strings.Compare(s.LogEntries[i].NodeIP, s.LogEntries[j].NodeIP) < 0
	}

	return bytes.Compare(ip1, ip2) < 0
}

func main() {
	// TODO split main into smaller functions
	nodetoolFile := flag.String("file", "", "Path to the nodetool status output file")
	datacenters := flag.String("datacenters", "", "Comma-separated list of datacenter names")
	listDCs := flag.Bool("list-dcs", false, "List all datacenters")
	sortOption := flag.String("sort", "date", "Sort by date, loglevel, linenumber, or nodeip")
	query := flag.String("query", "", "Comma-separated search terms in log entries")
	flag.Parse()

	if *nodetoolFile == "" || (*datacenters == "" && !*listDCs) || flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	if _, err := os.Stat(*nodetoolFile); os.IsNotExist(err) {
		log.Fatalf("File %s does not exist", *nodetoolFile)
	}

	file, err := os.Open(*nodetoolFile)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err = file.Close()
	}()

	nodes, err := ParseNodetoolStatus(file)
	if err != nil {
		log.Fatalf("Error while parsing the nodetool status output: %v", err)
	}

	if *listDCs {
		PrintDatacenters(nodes)
		return
	}

	// determine topLevelDir from nodetoolFile path
	topLevelDir := flag.Arg(0)
	dcNames := strings.Split(*datacenters, ",")
	queries := strings.Split(*query, ",")
	filteredNodes := filterNodesByDatacenters(nodes, dcNames)

	sortFunctions := map[string]func(LogEntries){
		"date":       func(entries LogEntries) { sort.Sort(ByDate{entries}) },
		"loglevel":   func(entries LogEntries) { sort.Sort(ByLogLevel{entries}) },
		"linenumber": func(entries LogEntries) { sort.Sort(ByLineNumber{entries}) },
		"nodeip":     func(entries LogEntries) { sort.Sort(ByNodeIP{entries}) },
	}

	sortFunc, ok := sortFunctions[*sortOption]
	if !ok {
		log.Fatalf("Invalid sort option: %s", *sortOption)
	}

	var wg sync.WaitGroup
	logEntryChan := make(chan *LogEntry, len(filteredNodes))

	for _, node := range filteredNodes {
		wg.Add(1)
		go func(node Node) {
			defer wg.Done()
			err := ProcessFile(node, topLevelDir, queries, logEntryChan)
			if err != nil {
				log.Printf("Error while processing logs for node %s: %v\n", node.Address, err)
			}
		}(node)
	}

	go func() {
		wg.Wait()
		close(logEntryChan)
	}()

	// create logEntries slice
	var logEntries LogEntries
	for entry := range logEntryChan {
		logEntries = append(logEntries, entry)
	}

	// use sortFunc to sort logEntries
	sortFunc(logEntries)

	for _, entry := range logEntries {
		fmt.Printf("%s:%s:%d: %v [%s] %s\n", entry.NodeIP, entry.FilePath, entry.LineNumber, entry.LogLevel, entry.Date, entry.Message)
	}
}

// ParseNodetoolStatus parses the output of nodetool status.
func ParseNodetoolStatus(r io.Reader) ([]Node, error) {
	//TODO implement being able to tell the status of the node (UN, DN, DL, UL, DJ, Uj, UM) and make it apart of the Node struct

	scanner := bufio.NewScanner(r)
	var nodes []Node
	var datacenter string
	var foundNodeStatus bool

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "Datacenter:"):
			fields := strings.Fields(line)
			if len(fields) > 1 {
				datacenter = fields[1]
			}
		case strings.HasPrefix(line, "UN") || strings.HasPrefix(line, "DN") || strings.HasPrefix(line, "UL") || strings.HasPrefix(line, "DL") || strings.HasPrefix(line, "UU") || strings.HasPrefix(line, "UJ") || strings.HasPrefix(line, "UM"):
			fields := strings.Fields(line)
			if len(fields) > 1 {
				nodes = append(nodes, Node{Address: fields[1], Datacenter: datacenter})
				foundNodeStatus = true
			}
		}
	}

	if !foundNodeStatus {
		return nil, fmt.Errorf("No nodes found in nodetool status output")
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nodes, nil
}

// ParseDate parses a date string in the format "2006-01-02 15:04:05,000".
func ParseDate(dateTimeStr string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05,000", dateTimeStr)
}

// ParseLogLevel parses a log level string into an iota.
func ParseLogLevel(logLevelStr string) (LogLevel, error) {
	switch logLevelStr {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARN":
		return WARN, nil
	case "ERROR":
		return ERROR, nil
	default:
		return 0, fmt.Errorf("Invalid log level: %s", logLevelStr)
	}
}

// ProcessFile processes a log file.
func ProcessFile(node Node, topLevelDir string, queries []string, logEntryChan chan *LogEntry) error {
	logDir := filepath.Join(topLevelDir, "nodes", node.Address, "logs", "cassandra")
	logFile := filepath.Join(logDir, "system.log")
	file, err := os.Open(logFile)
	if err != nil {
		return err
	}
	defer func() {
		err = file.Close()
	}()
	scanner := bufio.NewScanner(file)
	var currentEntry *LogEntry

	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := scanner.Text()

		if currentEntry != nil && !startsWithLogLevel(line) {
			currentEntry.Message += "\n" + line
			continue
		}

		if currentEntry != nil && matchQuery(currentEntry, queries) {
			logEntryChan <- currentEntry
		}

		currentEntry, err = ProcessLine(line, lineNumber, logFile)
		if err != nil {
			continue
		}
	}

	if currentEntry != nil && matchQuery(currentEntry, queries) {
		logEntryChan <- currentEntry
	}
	return scanner.Err()
}

// ProcessLine processes a line of a log file.
func ProcessLine(line string, lineNumber int, filePath string) (*LogEntry, error) {
	logLevelRegex := regexp.MustCompile(`^(\w+)\s`)
	logLevelMatch := logLevelRegex.FindStringSubmatch(line)

	if logLevelMatch == nil {
		return nil, nil
	}

	logLevel, err := ParseLogLevel(logLevelMatch[1])
	if err != nil {
		return nil, err
	}

	dateTimeRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2},\d{3})`)
	dateTimeMatch := dateTimeRegex.FindStringSubmatch(line)

	if dateTimeMatch == nil {
		return nil, nil
	}

	date, err := ParseDate(dateTimeMatch[1])
	if err != nil {
		return nil, err
	}

	return &LogEntry{
		LogLevel:   logLevel,
		Date:       date,
		LineNumber: lineNumber,
		NodeIP:     "",
		FilePath:   filePath,
		Message:    line,
	}, err
}

// PrintDatacenters prints the datacenters in the nodetool status output.
func PrintDatacenters(nodes []Node) {
	dcSet := make(map[string]struct{})
	for _, node := range nodes {
		dcSet[node.Datacenter] = struct{}{}
	}

	fmt.Println("Datacenters:")
	for dc := range dcSet {
		fmt.Println(dc)
	}
}

// filterNodesByDatacenters filters nodes by datacenters.
func filterNodesByDatacenters(nodes []Node, datacenters []string) []Node {
	var filteredNodes []Node
	dcSet := make(map[string]struct{})

	for _, dc := range datacenters {
		dcSet[dc] = struct{}{}
	}

	for _, node := range nodes {
		if _, ok := dcSet[node.Datacenter]; ok {
			filteredNodes = append(filteredNodes, node)
		}
	}

	return filteredNodes
}

// startsWithLogLevel returns true if the line starts with a log level.
func startsWithLogLevel(line string) bool {
	logLevelRegex := regexp.MustCompile(`^\w+\s`)
	return logLevelRegex.MatchString(line)
}

// matchQuery returns true if the log entry matches the query.
func matchQuery(entry *LogEntry, queries []string) bool {
	if len(queries) == 0 {
		return true
	}

	textToSearch := entry.Message

	for _, query := range queries {
		if strings.Contains(textToSearch, query) {
			textToSearch = strings.SplitN(textToSearch, query, 2)[1]
		} else {
			return false
		}
	}
	return true
}
