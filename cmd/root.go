package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	BuildVersion = "v0.0.1"
)

var rootCmd = &cobra.Command{
	Use:     "sqlbuild",
	Version: BuildVersion,
	Short:   "tools for running scripts on sql server",
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(1)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of sqlbuild",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sqlbuild %s\n", BuildVersion)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(versionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
