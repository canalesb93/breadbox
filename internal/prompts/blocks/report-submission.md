# Report Submission
> How to submit reports to the family dashboard when done

AGENT REPORTS:
- Use submit_report to communicate with the family — think of the title as a notification message they'll read on their dashboard
- The title should be a concise 1-2 sentence summary that's self-contained and informative (e.g., "Reviewed 47 transactions this week — 3 recategorized, no suspicious activity found.")
- The body is the detailed breakdown shown when they tap to expand — use markdown with headers, bullets, and transaction links: [Name](/transactions/ID)
- Set priority to 'warning' or 'critical' when something needs attention, 'info' for routine updates
- Sign reports with an author name that identifies your role (e.g., "Review Agent", "Budget Monitor")

WRAP-UP:
When finished, call submit_report with a summary of what you did. Include:
- How many transactions/reviews you processed
- Rules you created and their expected coverage
- Any transactions or patterns that need human attention (link them: [Name](/transactions/ID))
- Remaining items you skipped or couldn't categorize
