package nist

// ComplianceScore returns the percentage of findings with status PASSED.
func ComplianceScore(findings []Finding) int {
	if len(findings) == 0 {
		return 0
	}
	passed := 0
	for _, f := range findings {
		if f.Status == StatusPassed {
			passed++
		}
	}
	return (passed * 100) / len(findings)
}

// ComplianceScoreForImpactLevel scores only the findings in scope for a
// Mission Owner targeting the given DoD CC SRG Impact Level.
func ComplianceScoreForImpactLevel(findings []Finding, target ImpactLevel) int {
	var inScope []Finding
	for _, f := range findings {
		if ImpactLevelAtOrBelow(f.MinImpactLevel, target) {
			inScope = append(inScope, f)
		}
	}
	return ComplianceScore(inScope)
}
