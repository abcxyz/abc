// package flags contains flags that are commonly used by several commands
package flags

type CommonFlags struct {
	// Positional arguments:

	// Source is the location of the input template to be rendered.
	//
	// Example: github.com/abcxyz/abc/t/rest_server@latest
	Source string

	// GitProtocol is either https or ssh.
	GitProtocol string
}
