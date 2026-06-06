// Package rdap wraps RDAP (Registration Data Access Protocol) and DNS
// lookups. Provides domain registration data plus a CNAME-based hosting
// hint that recognizes Vercel/Netlify/Heroku/Cloudflare Pages/AWS/GCP.
package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// DomainInfo is the unified shape we return — RDAP + DNS combined.
type DomainInfo struct {
	Domain        string   `json:"domain"`
	Registered    string   `json:"registered,omitempty"` // ISO date
	LastChanged   string   `json:"last_changed,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	Registrar     string   `json:"registrar,omitempty"`
	Status        []string `json:"status,omitempty"` // e.g. clientTransferProhibited
	Nameservers   []string `json:"nameservers,omitempty"`
	HostingHint   string   `json:"hosting_hint,omitempty"`  // human-readable hint
	HostingCNAME  string   `json:"hosting_cname,omitempty"` // raw CNAME target if any
	IPv4Addresses []string `json:"ipv4_addresses,omitempty"`
}

// Lookup queries RDAP and DNS for a domain in parallel and returns a
// unified record. domain should be the bare host (e.g. "stripe.com").
func (c *Client) Lookup(ctx context.Context, domain string) (*DomainInfo, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "www.")
	domain = strings.TrimRight(domain, "/")
	if domain == "" {
		return nil, errors.New("empty domain")
	}

	out := &DomainInfo{Domain: domain}

	// RDAP via rdap.org (which delegates to the right TLD server with redirects).
	if rdapInfo, err := c.fetchRDAP(ctx, domain); err == nil && rdapInfo != nil {
		out.Registered = rdapInfo.Registered
		out.LastChanged = rdapInfo.LastChanged
		out.ExpiresAt = rdapInfo.ExpiresAt
		out.Registrar = rdapInfo.Registrar
		out.Status = rdapInfo.Status
	}

	// DNS lookups.
	resolver := &net.Resolver{}
	{
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if cname, err := resolver.LookupCNAME(dctx, domain); err == nil {
			cname = strings.TrimRight(strings.ToLower(cname), ".")
			out.HostingCNAME = cname
			out.HostingHint = classifyHostingCNAME(cname)
		}
	}
	{
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if nses, err := resolver.LookupNS(dctx, domain); err == nil {
			for _, ns := range nses {
				out.Nameservers = append(out.Nameservers, strings.TrimRight(ns.Host, "."))
			}
		}
	}
	// Try IPv4 directly — if no CNAME hint matched, the IP block sometimes reveals the host.
	{
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if ips, err := resolver.LookupIPAddr(dctx, domain); err == nil {
			for _, ip := range ips {
				if v4 := ip.IP.To4(); v4 != nil {
					out.IPv4Addresses = append(out.IPv4Addresses, v4.String())
				}
			}
		}
	}

	// If still no hosting hint, try www subdomain CNAME.
	if out.HostingHint == "" {
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if cname, err := resolver.LookupCNAME(dctx, "www."+domain); err == nil {
			cname = strings.TrimRight(strings.ToLower(cname), ".")
			if hint := classifyHostingCNAME(cname); hint != "" {
				out.HostingCNAME = cname
				out.HostingHint = hint
			}
		}
	}

	// Nameserver-based hint as a last resort.
	if out.HostingHint == "" && len(out.Nameservers) > 0 {
		if hint := classifyHostingFromNS(out.Nameservers); hint != "" {
			out.HostingHint = hint
		}
	}

	return out, nil
}

type rdapResponse struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Entities []struct {
		Roles      []string          `json:"roles"`
		VCardArray []json.RawMessage `json:"vcardArray"`
		Handle     string            `json:"handle"`
	} `json:"entities"`
	Status []string `json:"status"`
}

type rdapInfo struct {
	Registered  string
	LastChanged string
	ExpiresAt   string
	Registrar   string
	Status      []string
}

func (c *Client) fetchRDAP(ctx context.Context, domain string) (*rdapInfo, error) {
	u := "https://rdap.org/domain/" + domain
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rdap+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rdap %d", resp.StatusCode)
	}
	var raw rdapResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	out := &rdapInfo{Status: raw.Status}
	for _, e := range raw.Events {
		switch e.Action {
		case "registration":
			out.Registered = e.Date
		case "last changed":
			out.LastChanged = e.Date
		case "expiration":
			out.ExpiresAt = e.Date
		}
	}
	for _, ent := range raw.Entities {
		for _, role := range ent.Roles {
			if role == "registrar" {
				// vCard array shape is opaque; just report the handle if usable.
				if ent.Handle != "" {
					out.Registrar = ent.Handle
				}
			}
		}
	}
	return out, nil
}

// classifyHostingCNAME maps a CNAME target to a recognizable host name.
// Empty return = unknown.
func classifyHostingCNAME(cname string) string {
	c := strings.ToLower(cname)
	switch {
	case strings.HasSuffix(c, ".vercel-dns.com") || strings.HasSuffix(c, "vercel.app") || c == "cname.vercel-dns.com":
		return "Vercel"
	case strings.HasSuffix(c, ".netlify.app") || strings.Contains(c, "netlify.com"):
		return "Netlify"
	case strings.Contains(c, "herokuapp.com") || strings.Contains(c, "herokudns.com"):
		return "Heroku"
	case strings.HasSuffix(c, ".pages.dev"):
		return "Cloudflare Pages"
	case strings.HasSuffix(c, ".workers.dev"):
		return "Cloudflare Workers"
	case strings.Contains(c, "cloudfront.net"):
		return "AWS CloudFront"
	case strings.Contains(c, "elasticbeanstalk.com"):
		return "AWS Elastic Beanstalk"
	case strings.Contains(c, "amazonaws.com"):
		return "AWS"
	case strings.HasSuffix(c, ".firebaseapp.com") || strings.HasSuffix(c, ".web.app"):
		return "Firebase Hosting"
	case strings.Contains(c, "googleusercontent.com") || strings.Contains(c, "appspot.com"):
		return "Google Cloud"
	case strings.HasSuffix(c, ".azurewebsites.net"):
		return "Azure App Service"
	case strings.HasSuffix(c, ".github.io"):
		return "GitHub Pages"
	case strings.HasSuffix(c, ".fly.dev"):
		return "Fly.io"
	case strings.HasSuffix(c, ".onrender.com"):
		return "Render"
	case strings.HasSuffix(c, ".railway.app"):
		return "Railway"
	case strings.HasSuffix(c, ".webflow.io"):
		return "Webflow"
	case strings.HasSuffix(c, ".framer.app") || strings.HasSuffix(c, ".framer.website"):
		return "Framer"
	case strings.Contains(c, "shopifycdn.com") || strings.HasSuffix(c, ".shopify.com"):
		return "Shopify"
	case strings.HasSuffix(c, ".squarespace.com"):
		return "Squarespace"
	case strings.HasSuffix(c, ".wpengine.com") || strings.Contains(c, "wordpress.com"):
		return "WordPress"
	case strings.Contains(c, "fastly.net"):
		return "Fastly"
	}
	return ""
}

func classifyHostingFromNS(nses []string) string {
	for _, ns := range nses {
		ns = strings.ToLower(ns)
		switch {
		case strings.HasSuffix(ns, ".cloudflare.com"):
			return "Cloudflare DNS"
		case strings.Contains(ns, "awsdns"):
			return "AWS Route 53"
		case strings.Contains(ns, "googledomains") || strings.Contains(ns, "google.com"):
			return "Google DNS"
		case strings.Contains(ns, "azure-dns"):
			return "Azure DNS"
		case strings.Contains(ns, "vercel-dns") || strings.Contains(ns, "vercel.app"):
			return "Vercel"
		}
	}
	return ""
}
