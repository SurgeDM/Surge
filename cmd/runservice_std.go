//go:build !android

package cmd

func RunService() error {
	s, err := GetService()
	if err != nil {
		return rootCmd.Execute()
	}
	return s.Run()
}
