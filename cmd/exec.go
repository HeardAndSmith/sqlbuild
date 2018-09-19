package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mssqlbatch "github.com/denisenkom/go-mssqldb/batch"
	"github.com/heardandsmith/sqlbuild/lib"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [FILES, FOLDERS...]",
	Short: "Executes the passed sql scripts.",
	Args:  cobra.MinimumNArgs(1),
	Run:   execCmdRun,
}

func execCmdRun(cmd *cobra.Command, args []string) {
	log.SetFlags(0) // Disable timestamp on log messages.

	acceptEula := os.Getenv("ACCEPT_EULA")
	if acceptEula != "Y" {
		log.Fatal(`Environment variable ACCEPT_EULA must be Y.`)
	}

	password := os.Getenv("SA_PASSWORD")
	if password == "" {
		log.Fatal("Required environment variable SA_PASSWORD was not set.")
	}
	if err := lib.ValidatePassword(password); err != nil {
		log.Fatalf("Password in SA_PASSWORD was invalid: %s", err)
	}

	scripts, err := loadScriptsFromArgs(args)
	if err != nil {
		log.Fatalf("Error loading scripts: %s", err)
	}

	if len(scripts) == 0 {
		log.Fatal("Nothing to execute, exiting.")
	}

	if len(scripts) == 1 {
		log.Println("1 sql script found.")
	} else {
		log.Printf("%d sql scripts found.", len(scripts))
	}
	for _, filename := range scripts {
		log.Printf("  %s", filename)
	}

	log.Println("Starting sql server...")
	sqlsrv, err := lib.StartSqlServer(lib.SqlServerEnv{
		SA_PASSWORD: password,
		MSSQL_PID:   os.Getenv("MSSQL_PID"),
	})
	if err != nil {
		log.Fatal("Error starting sql server:\n" + err.Error())
	}
	sqlsrv.ExitOnUnexpectedShutdown()

	log.Println("Connecting to sql server...")
	conn, err := lib.NewConn(password)
	if err != nil {
		log.Fatalf("Error creating connection to server instance: %s", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Closing connection failed unexpectedly: %s", err)
		}
	}()

	var reset bool
	for _, filename := range scripts {
		if reset {
			if err := conn.ResetSession(); err != nil {
				log.Fatalf("Error resetting session between scripts: %s", err)
			}
		}
		name := baseNoExt(filename)

		batches, err := loadBatchesFromFile(filename)
		if err != nil {
			log.Fatalf("[%s]: Error loading batches: %s", name, err)
		}

		log.Printf("[%s]: %d batches", name, len(batches))

		start := time.Now()
		for i, query := range batches {
			complete := int(100 * float64(i) / float64(len(batches)))
			log.Printf("[%s]: %5d/%-5d %3d%%",
				name,
				i+1,
				len(batches),
				complete,
			)
			if err := conn.Exec(query); err != nil {
				log.Fatalf(
					"--------------- QUERY ---------------\n"+
						"%s\n"+
						"-------------------------------------\n"+
						"Error executing query in %#v, batch #%d/%d:\n"+
						"%s",
					query,
					filename,
					i+1,
					len(batches),
					err,
				)
			}
		}

		log.Printf("[%s]: completed in %s", name, time.Since(start))
		reset = true
	}
	log.Println("All scripts executed successfully!")

	log.Println("Shutting down sql server...")
	if err := sqlsrv.Shutdown(); err != nil {
		log.Fatalf("Error shutting down sql server: %s", err)
	}
}

// loadScriptsFromArgs parses the passed args. If the arg is a file, it is
// added directly, and if the arg is a directory, its contents are added (if
// they have a .sql extension).
func loadScriptsFromArgs(args []string) ([]string, error) {
	var scripts []string
	for _, filename := range args {
		info, err := os.Stat(filepath.Clean(filename))
		if err != nil {
			return nil, fmt.Errorf("Error reading %#v: %s", filename, err)
		}
		if info.IsDir() {
			folderscripts, err := loadScriptsFromDir(filename)
			if err != nil {
				return nil, fmt.Errorf("Error reading %#v: %s", filename, err)
			}
			scripts = append(scripts, folderscripts...)
		} else {
			scripts = append(scripts, filename)
		}
	}
	return scripts, nil
}

// loadScriptsFromDir returns all of the files with a .sql extension in the
// passed directory. Subdirectories aren't traversed. The filenames are
// returned sorted.
func loadScriptsFromDir(dir string) ([]string, error) {
	stats, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// Filter out directories, files without a .sql extension.
	var filtered []string
	for _, f := range stats {
		if f.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name()))
		if ext == ".sql" {
			filtered = append(filtered, filepath.Join(dir, f.Name()))
		}
	}

	// Sort them, which determines execution order.
	sort.Strings(filtered)
	return filtered, nil
}

// Returns the filename without any directories and with no extension.
func baseNoExt(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// Reads the file from the passed filename and splits its contents into batches
// around the "GO" statement.
func loadBatchesFromFile(filename string) ([]string, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	batches := mssqlbatch.Split(string(contents), "GO")
	return batches, nil
}
