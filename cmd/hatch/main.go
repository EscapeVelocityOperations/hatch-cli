package main

import (
    "fmt"
    "github.com/spf13/cobra"
)

func main() {
    rootCmd := &cobra.Command{
        Use:   "hatch",
        Short: "Hatch CLI - Developer tools for Hatch",
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Println("Hatch CLI")
        },
    }
    
    rootCmd.Execute()
}
