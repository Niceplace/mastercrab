/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package daily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dailyCmd represents the daily command
var DailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("daily called")
		//fmt.Println("Linear token is", viper.Get("linear.apiToken"))
		jsonData := map[string]string{
			"query": `
				query MyAssignedIssues { 
					viewer {
						assignedIssues(filter: { updatedAt: { gte: "-P4D" }}) {						
							edges { 
								node { 
									id title url 
								} 
							}
						} 
					}
				}
			`,
			"operationName": "MyAssignedIssues",
		}
		jsonValue, _ := json.Marshal(jsonData)
		request, err := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(jsonValue))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", viper.GetString("linear.apiToken"))
		//fmt.Printf("Request %s\n", request)
		if err != nil {
			fmt.Printf("Failed to create request %s\n", err)
		}

	},
}

func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dailyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	DailyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
