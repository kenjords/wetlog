package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestParseNodetoolStatus(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		wantNodes []Node
		wantError bool
	}{
		{
			name:  "single node up",
			input: "Datacenter: DC1\nUN 127.0.0.1\n",
			wantNodes: []Node{
				{Address: "127.0.0.1", Datacenter: "DC1"},
			},
			wantError: false,
		},
		{
			name:  "multiple nodes",
			input: "Datacenter: DC1\nUN 127.0.0.1\nDN 127.0.0.2\n",
			wantNodes: []Node{
				{Address: "127.0.0.1", Datacenter: "DC1"},
				{Address: "127.0.0.2", Datacenter: "DC1"},
			},
			wantError: false,
		},
		{
			name:  "multiple datacenters",
			input: "Datacenter: DC1\nUN 127.0.0.1\nUN 127.0.0.2\nDatacenter: DC2\nUN 127.0.1.1\n",
			wantNodes: []Node{
				{Address: "127.0.0.1", Datacenter: "DC1"},
				{Address: "127.0.0.2", Datacenter: "DC1"},
				{Address: "127.0.1.1", Datacenter: "DC2"},
			},
			wantError: false,
		},
		{
			name:  "node down, node up, node joining, node moving, node leaving",
			input: "Datacenter: DC1\nDN 127.0.0.1\nUN 127.0.0.2\nUJ 127.0.0.3\nUM 127.0.0.4\nUL 127.0.0.5\n",
			wantNodes: []Node{
				{Address: "127.0.0.1", Datacenter: "DC1"},
				{Address: "127.0.0.2", Datacenter: "DC1"},
				{Address: "127.0.0.3", Datacenter: "DC1"},
				{Address: "127.0.0.4", Datacenter: "DC1"},
				{Address: "127.0.0.5", Datacenter: "DC1"},
			},
			wantError: false,
		},
		{
			name:      "bad format",
			input:     "bad input format\n",
			wantNodes: nil,
			wantError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := strings.NewReader(tc.input)
			nodes, err := ParseNodetoolStatus(r)

			if (err != nil) != tc.wantError {
				t.Fatalf("parseNodetoolStatus() error = %v, wantErr %v", err, tc.wantError)
			}

			if !reflect.DeepEqual(nodes, tc.wantNodes) {
				t.Errorf("parseNodetoolStatus() = %v, want %v", nodes, tc.wantNodes)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:    "valid date",
			input:   "2023-07-13 12:01:01,000",
			want:    time.Date(2023, 7, 13, 12, 0o1, 0o1, 0, time.UTC),
			wantErr: false,
		},
		{
			name:    "invalid date",
			input:   "2023-13-07 12:01:01,000",
			want:    time.Time{},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDate(tc.input)

			if (err != nil) != tc.wantErr {
				t.Fatalf("parseDate() error = %v, wantErr %v", err, tc.wantErr)
			}

			if !got.Equal(tc.want) {
				t.Errorf("parseDate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    LogLevel
		wantErr bool
	}{
		{
			name:    "DEBUG log level",
			input:   "DEBUG",
			want:    DEBUG,
			wantErr: false,
		},
		{
			name:    "INFO log level",
			input:   "INFO",
			want:    INFO,
			wantErr: false,
		},
		{
			name:    "WARN log level",
			input:   "WARN",
			want:    WARN,
			wantErr: false,
		},
		{
			name:    "ERROR log level",
			input:   "ERROR",
			want:    ERROR,
			wantErr: false,
		},
		{
			name:    "Invalid log level",
			input:   "INVALID",
			want:    0,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLogLevel(tc.input)

			if (err != nil) != tc.wantErr {
				t.Fatalf("parseLogLevel() error = %v, wantErr %v", err, tc.wantErr)
			}

			if got != tc.want {
				t.Errorf("parseLogLevel() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProcessFile(t *testing.T) {
	t.Helper()
	node := Node{Address: "192.0.2.0"} // This IP address is a placeholder. It should be replaced with a valid IP.
	topLevelDir := "test"
	queries := []string{"INFO"}

	// Create the necessary path and file for the test
	testPath := filepath.Join(topLevelDir, "nodes", node.Address, "logs", "cassandra")
	err := os.MkdirAll(testPath, os.ModePerm)
	if err != nil {
		t.Fatalf("Couldn't create path: %v", err)
	}

	// Write sample data to the system.log file
	testFilePath := filepath.Join(testPath, "system.log")
	err = os.WriteFile(testFilePath, []byte("Sample log data"), 0o644)
	if err != nil {
		t.Fatalf("Couldn't write to file: %v", err)
	}

	// Make sure to clean up after test
	defer func() {
		err := os.RemoveAll(filepath.Join(topLevelDir, "nodes"))
		if err != nil {
			t.Errorf("Couldn't clean up test files: %v", err)
		}
	}()

	logEntryChan := make(chan *LogEntry)
	errChan := make(chan error)

	go func() {
		err := ProcessFile(node, topLevelDir, queries, logEntryChan)
		if err != nil {
			errChan <- err
		}
		close(logEntryChan)
	}()

	// Check whether it sends LogEntry to the channel correctly.
	for logEntry := range logEntryChan {
		if logEntry.Message == "" {
			t.Fatalf("Expected message in log entry, got empty")
		}
	}

	// Check for errors
	select {
	case err := <-errChan:
		t.Fatalf("ProcessFile() error = %v", err)
	default:
	}
}

func TestProcessLine(t *testing.T) {
	// Define test cases
	testCases := []struct {
		line      string
		lineNum   int
		filePath  string
		expectErr bool
	}{
		{
			line:      "INFO  [Solr TTL scheduler-0] 2023-07-05 13:03:37,128  AbstractSolrSecondaryIndex.java:1964 - Expired 3 documents in 18 milliseconds for core poms_om_search.om_mail_order_by_customer",
			lineNum:   1,
			filePath:  "testFilePath",
			expectErr: false,
		},
		{
			line:      "WARN  2023-04-24 12:12:32,430 org.apache.hadoop.hive.conf.HiveConf: HiveConf hive.server2.thrift.http.port expects INT type value",
			lineNum:   2,
			filePath:  "testFilePath",
			expectErr: false,
		},
		{
			line:      "INVALID  [Solr TTL scheduler-0] 2023-07-05 13:03:37,128  AbstractSolrSecondaryIndex.java:1964 - Expired 3 documents in 18 milliseconds for core poms_om_search.om_mail_order_by_customer",
			lineNum:   3,
			filePath:  "testFilePath",
			expectErr: true, // expect error due to invalid log level
		},
	}

	for i, testCase := range testCases {
		_, err := ProcessLine(testCase.line, testCase.lineNum, testCase.filePath)

		if err != nil && !testCase.expectErr {
			t.Errorf("Test case %d: unexpected error: %v", i+1, err)
		} else if err == nil && testCase.expectErr {
			t.Errorf("Test case %d: expected error but got none", i+1)
		}
	}
}

func TestPrintDatacenters(t *testing.T) {
	nodes := []Node{
		{Address: "192.168.1.1", Datacenter: "DC1"},
		{Address: "192.168.1.2", Datacenter: "DC2"},
		{Address: "192.168.1.3", Datacenter: "DC1"},
		{Address: "192.168.1.4", Datacenter: "DC3"},
		{Address: "192.168.1.5", Datacenter: "DC2"},
	}

	expectedOutput := "Datacenters:\nDC1\nDC2\nDC3\n"

	// Create a pipe for capturing the output
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function
	PrintDatacenters(nodes)

	// Reset the standard output
	err := w.Close()
	if err != nil {
		t.Fatalf("Couldn't close pipe: %v", err)
	}
	os.Stdout = os.NewFile(1, "")

	// Read the captured output from the pipe
	capturedOutput, _ := io.ReadAll(r)
	output := string(capturedOutput)

	if output != expectedOutput {
		t.Errorf("Expected output:\n%s\nGot:\n%s", expectedOutput, output)
	}
}

func TestFilterNodesByDatacenters(t *testing.T) {
	// Define test nodes and datacenters
	nodes := []Node{
		{"192.168.1.1", "dc1"},
		{"192.168.1.2", "dc1"},
		{"192.168.1.3", "dc2"},
		{"192.168.1.4", "dc3"},
		{"192.168.1.5", "dc4"},
	}
	datacenters := []string{"dc1", "dc3"}

	// Run the filter function
	result := filterNodesByDatacenters(nodes, datacenters)

	// Expected result
	expected := []Node{
		{"192.168.1.1", "dc1"},
		{"192.168.1.2", "dc1"},
		{"192.168.1.4", "dc3"},
	}

	// Check if result matches expected
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("filterNodesByDatacenters() = %v, want %v", result, expected)
	}
}

func TestStartsWithLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "Line starts with log level",
			line:     "INFO  [main] 2023-07-14 16:00:00,658 YamlConfigurationLoader.java:89 - Configuration location: file:/etc/cassandra/cassandra.yaml",
			expected: true,
		},
		{
			name:     "Line does not start with log level",
			line:     "[main] 2023-07-14 16:00:00,658 YamlConfigurationLoader.java:89 - Configuration location: file:/etc/cassandra/cassandra.yaml",
			expected: false,
		},
		{
			name:     "Empty line",
			line:     "",
			expected: false,
		},
		{
			name:     "Line starts with non-word characters",
			line:     "# INFO 2023-07-14 16:00:00,658 YamlConfigurationLoader.java:89 - Configuration location: file:/etc/cassandra/cassandra.yaml",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := startsWithLogLevel(test.line); got != test.expected {
				t.Errorf("startsWithLogLevel() = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestMatchQuery(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		entry    *LogEntry
		queries  []string
		expected bool
	}{
		{
			name: "No Queries",
			entry: &LogEntry{
				Message: "No queries to match",
			},
			queries:  []string{},
			expected: true,
		},
		{
			name: "Single Match",
			entry: &LogEntry{
				Message: "This is a test message",
			},
			queries:  []string{"test"},
			expected: true,
		},
		{
			name: "Single Non-Match",
			entry: &LogEntry{
				Message: "This is a test message",
			},
			queries:  []string{"non-match"},
			expected: false,
		},
		{
			name: "Multiple Matches",
			entry: &LogEntry{
				Message: "This is a test message with multiple queries to match",
			},
			queries:  []string{"test", "message", "multiple", "queries"},
			expected: true,
		},
		{
			name: "Multiple Queries with Non-Match",
			entry: &LogEntry{
				Message: "This is a test message with multiple queries to match",
			},
			queries:  []string{"test", "non-match"},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := matchQuery(tc.entry, tc.queries)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func TestLogEntriesSorting(t *testing.T) {
	// Create sample log entries
	entry1 := &LogEntry{
		LogLevel:   2,
		Date:       time.Now().Add(-time.Hour),
		LineNumber: 10,
		NodeIP:     "192.168.1.10",
		FilePath:   "/var/log/app.log",
		Message:    "Error occurred",
	}
	entry2 := &LogEntry{
		LogLevel:   1,
		Date:       time.Now(),
		LineNumber: 5,
		NodeIP:     "192.168.1.5",
		FilePath:   "/var/log/app.log",
		Message:    "Info message",
	}
	entry3 := &LogEntry{
		LogLevel:   3,
		Date:       time.Now().Add(-2 * time.Hour),
		LineNumber: 15,
		NodeIP:     "192.168.1.3",
		FilePath:   "/var/log/app.log",
		Message:    "Warning",
	}

	// Create a slice of log entries
	entries := LogEntries{entry1, entry2, entry3}

	// Sort by date
	sort.Sort(ByDate{entries})
	expectedDates := []time.Time{entry3.Date, entry1.Date, entry2.Date}
	for i, entry := range entries {
		if !reflect.DeepEqual(entry.Date, expectedDates[i]) {
			t.Errorf("Expected date %v at index %d, but got %v", expectedDates[i], i, entry.Date)
		}
	}

	// Sort by log level
	sort.Sort(ByLogLevel{entries})
	expectedLevels := []LogLevel{1, 2, 3}
	for i, entry := range entries {
		if entry.LogLevel != expectedLevels[i] {
			t.Errorf("Expected log level %d at index %d, but got %d", expectedLevels[i], i, entry.LogLevel)
		}
	}

	// Sort by line number
	sort.Sort(ByLineNumber{entries})
	expectedLineNumbers := []int{5, 10, 15}
	for i, entry := range entries {
		if entry.LineNumber != expectedLineNumbers[i] {
			t.Errorf("Expected line number %d at index %d, but got %d", expectedLineNumbers[i], i, entry.LineNumber)
		}
	}

	// Sort by node IP
	sort.Sort(ByNodeIP{entries})
	expectedIPs := []string{"192.168.1.3", "192.168.1.5", "192.168.1.10"}
	for i, entry := range entries {
		if entry.NodeIP != expectedIPs[i] {
			t.Errorf("Expected node IP %s at index %d, but got %s", expectedIPs[i], i, entry.NodeIP)
		}
	}
}

// TestLen tests the Len() method of the LogEntries.
func TestLen(t *testing.T) {
	entry1 := &LogEntry{
		LogLevel:   DEBUG,
		Date:       time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
		LineNumber: 1,
		NodeIP:     "192.168.1.1",
		FilePath:   "/var/log/test.log",
		Message:    "Debug message 1",
	}
	entry2 := &LogEntry{
		LogLevel:   INFO,
		Date:       time.Date(2023, 7, 14, 1, 0, 0, 0, time.UTC),
		LineNumber: 2,
		NodeIP:     "192.168.1.2",
		FilePath:   "/var/log/test.log",
		Message:    "Info message 2",
	}
	logEntries := LogEntries{entry2, entry1}

	if logEntries.Len() != 2 {
		t.Fatalf("Expected Len() to return 2, but got %v", logEntries.Len())
	}
}

// TestSwap tests the Swap() method of the LogEntries.
func TestSwap(t *testing.T) {

	entry1 := &LogEntry{
		LogLevel:   DEBUG,
		Date:       time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
		LineNumber: 1,
		NodeIP:     "192.168.1.1",
		FilePath:   "/var/log/test.log",
		Message:    "Debug message 1",
	}
	entry2 := &LogEntry{
		LogLevel:   INFO,
		Date:       time.Date(2023, 7, 14, 1, 0, 0, 0, time.UTC),
		LineNumber: 2,
		NodeIP:     "192.168.1.2",
		FilePath:   "/var/log/test.log",
		Message:    "Info message 2",
	}

	logEntries := LogEntries{entry2, entry1}
	logEntries.Swap(0, 1)
	if logEntries[0].Message != "Debug message 1" || logEntries[1].Message != "Info message 2" {
		t.Fatalf("Swap() did not swap the entries correctly")
	}
}

// TestByDate tests the sorting of LogEntries by date.
func TestByDate(t *testing.T) {

	entry1 := &LogEntry{
		LogLevel:   DEBUG,
		Date:       time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
		LineNumber: 1,
		NodeIP:     "192.168.1.1",
		FilePath:   "/var/log/test.log",
		Message:    "Debug message 1",
	}
	entry2 := &LogEntry{
		LogLevel:   INFO,
		Date:       time.Date(2023, 7, 14, 1, 0, 0, 0, time.UTC),
		LineNumber: 2,
		NodeIP:     "192.168.1.2",
		FilePath:   "/var/log/test.log",
		Message:    "Info message 2",
	}

	logEntries := LogEntries{entry2, entry1}
	sort.Sort(ByDate{logEntries})
	if !logEntries[0].Date.Before(logEntries[1].Date) {
		t.Fatalf("ByDate sort failed")
	}
}

// TestByLogLevel tests the sorting of LogEntries by log level.
func TestByLogLevel(t *testing.T) {

	entry1 := &LogEntry{
		LogLevel:   DEBUG,
		Date:       time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
		LineNumber: 1,
		NodeIP:     "192.168.1.1",
		FilePath:   "/var/log/test.log",
		Message:    "Debug message 1",
	}
	entry2 := &LogEntry{
		LogLevel:   INFO,
		Date:       time.Date(2023, 7, 14, 1, 0, 0, 0, time.UTC),
		LineNumber: 2,
		NodeIP:     "192.168.1.2",
		FilePath:   "/var/log/test.log",
		Message:    "Info message 2",
	}

	logEntries := LogEntries{entry2, entry1}
	sort.Sort(ByLogLevel{logEntries})
	if logEntries[0].LogLevel > logEntries[1].LogLevel {
		t.Fatalf("ByLogLevel sort failed")
	}
}

// TestByLineNumber tests the sorting of LogEntries by line number.
func TestByLineNumber(t *testing.T) {

	entry1 := &LogEntry{
		LogLevel:   DEBUG,
		Date:       time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
		LineNumber: 1,
		NodeIP:     "192.168.1.1",
		FilePath:   "/var/log/test.log",
		Message:    "Debug message 1",
	}
	entry2 := &LogEntry{
		LogLevel:   INFO,
		Date:       time.Date(2023, 7, 14, 1, 0, 0, 0, time.UTC),
		LineNumber: 2,
		NodeIP:     "192.168.1.2",
		FilePath:   "/var/log/test.log",
		Message:    "Info message 2",
	}

	logEntries := LogEntries{entry2, entry1}
	sort.Sort(ByLineNumber{logEntries})
	if logEntries[0].LineNumber > logEntries[1].LineNumber {
		t.Fatalf("ByLineNumber sort failed")
	}
}

// TestByNodeIP tests the sorting of LogEntries by node IP.
func TestByNodeIP(t *testing.T) {

	entry1 := &LogEntry{
		LogLevel:   DEBUG,
		Date:       time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
		LineNumber: 1,
		NodeIP:     "192.168.1.1",
		FilePath:   "/var/log/test.log",
		Message:    "Debug message 1",
	}
	entry2 := &LogEntry{
		LogLevel:   INFO,
		Date:       time.Date(2023, 7, 14, 1, 0, 0, 0, time.UTC),
		LineNumber: 2,
		NodeIP:     "192.168.1.2",
		FilePath:   "/var/log/test.log",
		Message:    "Info message 2",
	}

	logEntries := LogEntries{entry2, entry1}
	sort.Sort(ByNodeIP{logEntries})
	if logEntries[0].NodeIP > logEntries[1].NodeIP {
		t.Fatalf("ByNodeIP sort failed")
	}
}
