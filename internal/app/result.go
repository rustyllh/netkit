package app

// Check 表示一项可独立评估的诊断结果。
type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

// Result 表示可供机器读取的操作结果。
type Result struct {
	OK        bool     `json:"ok"`
	Operation string   `json:"operation"`
	Checks    []Check  `json:"checks"`
	Warnings  []string `json:"warnings,omitempty"`
}

func failed(name, detail string) Check {
	return Check{Name: name, OK: false, Severity: "error", Detail: detail}
}

func result(operation string, checks []Check) Result {
	ok := true
	for _, check := range checks {
		if !check.OK && check.Severity == "error" {
			ok = false
		}
	}
	return Result{OK: ok, Operation: operation, Checks: checks}
}
