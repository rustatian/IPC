module github.com/rustatian/ipc/tests

go 1.26.2

require (
	github.com/rustatian/ipc v1.1.0
	github.com/stretchr/testify v1.12.1
	golang.org/x/sys v0.49.0
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

replace github.com/rustatian/ipc => ../
