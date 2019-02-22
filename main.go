package main

import (
	"log"
	"os"

	"github.com/mikesimons/kpatch/pkg/kpatch"

	"github.com/spf13/cobra"
)

var versionString = "dev"

func main() {
	var selector string
	var merges []string
	var exprs []string

	cmd := &cobra.Command{
		Use:     "kpatch",
		Version: versionString,
		Run: func(cmd *cobra.Command, args []string) {
			err := kpatch.Run(args, selector, merges, exprs, os.Stdout)
			if err != nil {
				log.Fatalln(err)
			}
		},
	}

	cmd.Flags().StringVarP(&selector, "selector", "s", "", "Document selector to specify which to apply expressions / merges to.")
	cmd.Flags().StringArrayVarP(&merges, "merge", "m", merges, "YAML/JSON file or inline YAML/JSON to merge with selected documents. May be used more than once.")
	cmd.Flags().StringArrayVarP(&exprs, "action", "a", exprs, "Action expression to apply to selected documents. May be used more than once.")

	err := cmd.Execute()
	if err != nil {
		log.Fatalln("Error: ", err)
	}
}
