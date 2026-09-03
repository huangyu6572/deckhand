package wire

const ProtocolVersion = 1
const ProductVersion = "0.1.0"

const (
	KindRequest  = "request"
	KindResponse = "response"
	KindEvent    = "event"
)

const (
	JobRun           = "Job.Run"
	JobWait          = "Job.Wait"
	JobFollow        = "Job.Follow"
	JobCancel        = "Job.Cancel"
	JobList          = "Job.List"
	FileCopy         = "File.Copy"
	TargetList       = "Target.List"
	SessionOpen      = "Session.Open"
	SessionExec      = "Session.Exec"
	SessionRead      = "Session.Read"
	SessionWrite     = "Session.Write"
	SessionResize    = "Session.Resize"
	SessionAttach    = "Session.Attach"
	SessionDetach    = "Session.Detach"
	SessionClose     = "Session.Close"
	SerialList       = "Serial.List"
	SerialExec       = "Serial.Exec"
	SerialMonitor    = "Serial.Monitor"
	DeployStart      = "Deploy.Start"
	ConnectionList   = "Connection.List"
	ConnectionStatus = "Connection.Status"
	ConnectionClose  = "Connection.Close"
	SecretSet        = "Secret.Set"
	SecretDelete     = "Secret.Delete"
)
