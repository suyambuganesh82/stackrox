package postgres

//go:generate pg-table-bindings-wrapper --type=ProcessBaselineResults --table=processWhitelistResults --key-func=GetDeploymentId()
