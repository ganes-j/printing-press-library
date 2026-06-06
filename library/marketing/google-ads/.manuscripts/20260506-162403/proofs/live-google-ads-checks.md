# Live Google Ads Checks

Run ID: `20260506-162403`

The CLI was tested against live Google Ads OAuth credentials with the `https://www.googleapis.com/auth/adwords` scope and a valid developer token.

Passed checks:

- `customers list-accessible-customers --json --no-cache`
- `customers-google-ads search 3109226734 --query 'SELECT customer.id, customer.descriptive_name FROM customer LIMIT 1' --json --no-cache`
- `customers-google-ads search 3109226734 --query 'SELECT campaign.id, campaign.name, campaign.status FROM campaign LIMIT 1' --json --no-cache`
- `google-ads-fields search --query "SELECT name, category, data_type WHERE name LIKE 'campaign.%' LIMIT 5" --json --no-cache`

Credentials and returned account details are intentionally omitted from this proof.
