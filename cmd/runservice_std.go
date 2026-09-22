//go:build !android && !linux

package cmd

func RunService() error {
	s, err := GetService()
	if err != nil {
		return err
	}
	return s.Run()
}
