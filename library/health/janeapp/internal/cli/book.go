// Copyright 2026 Omar Shahine and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written write path: book / reschedule / cancel. These mutate real
// appointments, so every command is dry-run by default (prints the exact
// request it would send) and requires --confirm to actually submit. The write
// request shape follows Jane's REST conventions for patient booking; the
// concrete endpoint is verified against a live session before a real submit.

package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"janeapp-pp-cli/internal/client"
)

// buildAppointmentBody constructs the create-appointment request body. Jane's
// patient booking creates an appointment from a chosen opening; the body mirrors
// the opening fields plus the resolved timing.
func buildAppointmentBody(treatment, staff, location int, startAt string, duration int) map[string]any {
	appt := map[string]any{
		"treatment_id":    treatment,
		"staff_member_id": staff,
		"location_id":     location,
		"start_at":        startAt,
	}
	if duration > 0 {
		appt["duration"] = duration
	}
	return map[string]any{"appointment": appt}
}

// clientForWrite builds a client for the active clinic, forcing dry-run unless
// the user passed --confirm. Returns the clinic so callers can message clearly.
func clientForWrite(flags *rootFlags, confirm bool) (*client.Client, *Clinic, error) {
	clinic, err := requireActiveClinic(flags)
	if err != nil {
		return nil, nil, err
	}
	if clinic.Session == "" {
		return nil, nil, usageErr(fmt.Errorf("not logged in to clinic %q; run 'janeapp-pp-cli auth login --clinic %s'", clinic.Name, clinic.Name))
	}
	c, err := flags.newClient()
	if err != nil {
		return nil, nil, err
	}
	// Without --confirm, never send a real mutation: reuse the client's dry-run
	// path so the user sees exactly what would be submitted.
	if !confirm {
		c.DryRun = true
	}
	return c, clinic, nil
}

func writeResult(cmd *cobra.Command, flags *rootFlags, confirm bool, action string, data json.RawMessage, status int) error {
	if !confirm {
		fmt.Fprintf(cmd.ErrOrStderr(), "(dry run — no changes made. Re-run with --confirm to %s.)\n", action)
		if flags.asJSON {
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"dry_run": true, "action": action}, flags)
		}
		return nil
	}
	if flags.asJSON {
		out := map[string]any{"action": action, "status": status}
		if len(data) > 0 {
			var parsed any
			if json.Unmarshal(data, &parsed) == nil {
				out["result"] = parsed
			}
		}
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s (status %d).\n", action, status)
	if len(data) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}
	return nil
}

func newBookCmd(flags *rootFlags) *cobra.Command {
	var treatment, staff, location int
	var atStr string
	var confirm, debug bool

	cmd := &cobra.Command{
		Use:   "book",
		Short: "Book an appointment (dry-run by default; --confirm to submit)",
		Long: `Book an appointment at the active clinic. Requires a logged-in session.

Booking runs Jane's real reserve -> confirm transaction (holds the slot, then
confirms it under your patient profile). By default this is a DRY RUN: it prints
what it would book and changes nothing. Add --confirm to actually book.

Find IDs with 'treatments', 'staff', 'locations'; find a real open slot with
'next-opening' or 'openings'.`,
		Example:     "  janeapp-pp-cli book --clinic leahkangas --treatment 2 --staff 1 --location 1 --at 2026-08-21T16:00:00-07:00\n  janeapp-pp-cli book --clinic leahkangas --treatment 2 --staff 1 --location 1 --at 2026-08-21T16:00:00-07:00 --confirm",
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 && len(args) == 0 {
				return cmd.Help()
			}
			if flags.dryRun {
				return nil
			}
			if treatment <= 0 || staff <= 0 || location <= 0 || atStr == "" {
				return usageErr(fmt.Errorf("book requires --treatment, --staff, --location, and --at"))
			}
			if _, err := parseFlexibleDate(atStr); err != nil {
				return usageErr(fmt.Errorf("invalid --at %q: use RFC3339 or YYYY-MM-DDTHH:MM:SS", atStr))
			}
			clinic, err := requireActiveClinic(flags)
			if err != nil {
				return err
			}
			if clinic.Session == "" {
				return usageErr(fmt.Errorf("not logged in to clinic %q; run 'janeapp-pp-cli auth login --clinic %s --chrome'", clinic.Name, clinic.Name))
			}
			if !confirm {
				// Dry run: describe the transaction without touching Jane.
				plan := map[string]any{
					"dry_run": true, "clinic": clinic.Name, "action": "book",
					"treatment_id": treatment, "staff_member_id": staff,
					"location_id": location, "start_at": atStr,
					"steps": []string{
						"POST /api/v2/reservations (hold slot)",
						"POST /api/v2/appointments/{reservation_id}/book (confirm)",
					},
				}
				if flags.asJSON {
					return printJSONFiltered(cmd.OutOrStdout(), plan, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "DRY RUN — would book at %s:\n", clinic.BaseURL)
				fmt.Fprintf(cmd.OutOrStdout(), "  treatment=%d staff=%d location=%d start=%s\n", treatment, staff, location, atStr)
				fmt.Fprintln(cmd.OutOrStdout(), "  via: reserve slot -> confirm booking")
				fmt.Fprintln(cmd.ErrOrStderr(), "(no changes made. Re-run with --confirm to book.)")
				return nil
			}
			// Real booking.
			var dbg io.Writer
			if debug {
				dbg = cmd.ErrOrStderr()
			}
			booker, err := newJaneBooker(cmd.Context(), clinic, flags.timeout, dbg)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			result, err := booker.Book(cmd.Context(), treatment, staff, location, atStr)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if flags.asJSON {
				var parsed any
				if json.Unmarshal([]byte(result), &parsed) == nil {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"booked": true, "clinic": clinic.Name, "appointment": parsed}, flags)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Booked at %s (clinic %q).\n", atStr, clinic.Name)
			return nil
		},
	}
	cmd.Flags().IntVar(&treatment, "treatment", 0, "Treatment ID (see 'treatments')")
	cmd.Flags().IntVar(&staff, "staff", 0, "Practitioner ID (see 'staff')")
	cmd.Flags().IntVar(&location, "location", 0, "Location ID (see 'locations')")
	cmd.Flags().StringVar(&atStr, "at", "", "Appointment start time (RFC3339 or YYYY-MM-DDTHH:MM:SS)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Actually submit the booking (otherwise dry-run)")
	cmd.Flags().BoolVar(&debug, "debug", false, "Print the reserve/confirm HTTP trace")
	return cmd
}

func newRescheduleCmd(flags *rootFlags) *cobra.Command {
	var id, treatment, staff, location int
	var atStr string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "reschedule",
		Short: "Reschedule an existing appointment (dry-run by default; --confirm to submit)",
		Long: `Move an existing appointment to a new time. Requires a logged-in
session and the appointment's ID (see 'appointments upcoming'). Dry-run by
default; add --confirm to submit.`,
		Example:     "  janeapp-pp-cli reschedule --id 12345 --at 2026-07-20T10:00:00\n  janeapp-pp-cli reschedule --id 12345 --at 2026-07-20T10:00:00 --confirm",
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 && len(args) == 0 {
				return cmd.Help()
			}
			if flags.dryRun {
				return nil
			}
			if id <= 0 || atStr == "" {
				return usageErr(fmt.Errorf("reschedule requires --id and --at"))
			}
			if _, err := parseFlexibleDate(atStr); err != nil {
				return usageErr(fmt.Errorf("invalid --at %q", atStr))
			}
			c, _, err := clientForWrite(flags, confirm)
			if err != nil {
				return err
			}
			appt := map[string]any{"start_at": atStr}
			if treatment > 0 {
				appt["treatment_id"] = treatment
			}
			if staff > 0 {
				appt["staff_member_id"] = staff
			}
			if location > 0 {
				appt["location_id"] = location
			}
			body := map[string]any{"appointment": appt}
			data, status, err := c.Patch(cmd.Context(), fmt.Sprintf("/api/v2/appointments/%d", id), body)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			return writeResult(cmd, flags, confirm, "reschedule appointment", data, status)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Appointment ID to reschedule (see 'appointments upcoming')")
	cmd.Flags().StringVar(&atStr, "at", "", "New start time (RFC3339 or YYYY-MM-DDTHH:MM:SS)")
	cmd.Flags().IntVar(&treatment, "treatment", 0, "Optionally change the treatment ID")
	cmd.Flags().IntVar(&staff, "staff", 0, "Optionally change the practitioner ID")
	cmd.Flags().IntVar(&location, "location", 0, "Optionally change the location ID")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Actually submit the change (otherwise dry-run)")
	return cmd
}

func newCancelCmd(flags *rootFlags) *cobra.Command {
	var id int
	var confirm bool

	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel an appointment (dry-run by default; --confirm to submit)",
		Long: `Cancel an existing appointment by ID (see 'appointments upcoming').
Requires a logged-in session. Dry-run by default; add --confirm to actually
cancel. Note: clinics may restrict cancellations outside a notice window.`,
		Example:     "  janeapp-pp-cli cancel --id 12345\n  janeapp-pp-cli cancel --id 12345 --confirm",
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 && len(args) == 0 {
				return cmd.Help()
			}
			if flags.dryRun {
				return nil
			}
			if id <= 0 {
				return usageErr(fmt.Errorf("cancel requires --id (see 'appointments upcoming')"))
			}
			c, _, err := clientForWrite(flags, confirm)
			if err != nil {
				return err
			}
			data, status, err := c.Delete(cmd.Context(), fmt.Sprintf("/api/v2/appointments/%d", id))
			if err != nil {
				return classifyAPIError(err, flags)
			}
			return writeResult(cmd, flags, confirm, "cancel appointment", data, status)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Appointment ID to cancel (see 'appointments upcoming')")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Actually submit the cancellation (otherwise dry-run)")
	return cmd
}
