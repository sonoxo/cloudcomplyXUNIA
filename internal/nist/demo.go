package nist

// DemoFindings generates realistic fake findings across 6 control families
// for demo mode. When live Security Hub integration lands, this is the only
// function that needs replacing — callers (TUI, report) don't change.
func DemoFindings() []Finding {
	return []Finding{
		// AC — Access Control
		{"AC-2", "Account Management", "AC", StatusFailed, SeverityHigh, 12, "Assess", IL2},
		{"AC-2(1)", "Auto Temp/Emergency Accounts", "AC", StatusPassed, SeverityLow, 0, "Monitor", IL4},
		{"AC-3", "Access Enforcement", "AC", StatusPassed, SeverityMedium, 0, "Monitor", IL2},
		{"AC-6", "Least Privilege", "AC", StatusFailed, SeverityHigh, 23, "Assess", IL4},
		{"AC-17", "Remote Access", "AC", StatusFailed, SeverityMedium, 8, "Assess", IL2},

		// AU — Audit and Accountability
		{"AU-2", "Event Logging", "AU", StatusPassed, SeverityMedium, 0, "Monitor", IL2},
		{"AU-3", "Content of Audit Records", "AU", StatusFailed, SeverityMedium, 5, "Assess", IL2},
		{"AU-9", "Protection of Audit Information", "AU", StatusFailed, SeverityHigh, 15, "Assess", IL6},
		{"AU-12", "Audit Record Generation", "AU", StatusPassed, SeverityLow, 0, "Monitor", IL2},

		// CM — Configuration Management
		{"CM-2", "Baseline Configuration", "CM", StatusFailed, SeverityCritical, 47, "Implement", IL2},
		{"CM-6", "Configuration Settings", "CM", StatusFailed, SeverityHigh, 31, "Assess", IL4},
		{"CM-7", "Least Functionality", "CM", StatusPassed, SeverityMedium, 0, "Monitor", IL2},
		{"CM-8", "System Component Inventory", "CM", StatusFailed, SeverityMedium, 19, "Assess", IL2},
		{"CM-11", "User-Installed Software", "CM", StatusPassed, SeverityLow, 0, "Monitor", IL4},

		// IA — Identification and Authentication
		{"IA-2", "Identification and Authentication", "IA", StatusFailed, SeverityCritical, 38, "Assess", IL2},
		{"IA-2(1)", "MFA for Privileged Accounts", "IA", StatusFailed, SeverityCritical, 41, "Implement", IL4},
		{"IA-5", "Authenticator Management", "IA", StatusFailed, SeverityHigh, 22, "Assess", IL2},
		{"IA-8", "Identification (Non-Org Users)", "IA", StatusPassed, SeverityMedium, 0, "Monitor", IL2},

		// SC — System and Communications Protection
		{"SC-7", "Boundary Protection", "SC", StatusFailed, SeverityHigh, 14, "Assess", IL2},
		{"SC-8", "Transmission Confidentiality", "SC", StatusPassed, SeverityMedium, 0, "Monitor", IL4},
		{"SC-12", "Cryptographic Key Establishment", "SC", StatusFailed, SeverityMedium, 9, "Assess", IL5},
		{"SC-28", "Protection of Info at Rest", "SC", StatusFailed, SeverityHigh, 27, "Assess", IL4},
		{"SC-28(1)", "Cryptographic Protection", "SC", StatusPassed, SeverityMedium, 0, "Monitor", IL5},

		// SI — System and Information Integrity
		{"SI-2", "Flaw Remediation", "SI", StatusFailed, SeverityHigh, 33, "Assess", IL2},
		{"SI-3", "Malicious Code Protection", "SI", StatusPassed, SeverityMedium, 0, "Monitor", IL2},
		{"SI-4", "System Monitoring", "SI", StatusFailed, SeverityMedium, 7, "Monitor", IL4},
		{"SI-7", "Software and Firmware Integrity", "SI", StatusPassed, SeverityLow, 0, "Monitor", IL4},
	}
}
