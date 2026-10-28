package cmd

import (
	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/spf13/cobra"
)

// Keep parser failures inside the same exit-code contract as runtime failures.
func configureContractValidation(command *cobra.Command) {
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.NewError(clioutput.InvalidArguments, "Invalid command flags; see --help.", err)
	})
	validate := command.Args
	if validate != nil {
		command.Args = func(cmd *cobra.Command, args []string) error {
			if err := validate(cmd, args); err != nil {
				if clioutput.Describe(err).Code == clioutput.InvalidArguments {
					return err
				}
				return clioutput.NewError(clioutput.InvalidArguments, "Invalid command arguments; see --help.", err)
			}
			return nil
		}
	}
}
