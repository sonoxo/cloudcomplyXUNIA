package awsclient

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"

	"cloudcomply/internal/nist"
)

// GetOrgSummary fetches the org identifier and total account count.
// AWS Organizations has no "display name" concept for an organization, so
// the org's ID (e.g. "o-abc123xyz") is used in place of demo mode's
// fabricated "Acme Federal Org" name.
func (c *Client) GetOrgSummary(ctx context.Context) (nist.OrgSummary, error) {
	desc, err := c.org.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return nist.OrgSummary{}, fmt.Errorf("describe organization: %w", err)
	}

	var count int
	p := organizations.NewListAccountsPaginator(c.org, &organizations.ListAccountsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nist.OrgSummary{}, fmt.Errorf("list accounts: %w", err)
		}
		count += len(page.Accounts)
	}

	return nist.OrgSummary{
		Name:         aws.ToString(desc.Organization.Id),
		AccountCount: count,
	}, nil
}
