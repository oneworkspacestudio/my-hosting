# Hosted WonderPal process resilience

User requires reliable deployment and recovery after reboots/interruption. The
public WonderPal app is a PC2 Hosting Node project; its learning-only policy and
existing resident voice grant must stay intact. No owner installation is a
development fixture. Existing source67ba8d4 includes unreleased preview3 HTML
entry support; preview4 retains it and needs corresponding regression coverage.

Found a permanent five-exit supervisor cutoff, no health check for a hung live
process, restoration only while Caddy is absent, and one failed project aborting
restoration of all routes. Preview4 removes that cutoff, caps backoff30seconds,
resets after stable running, probes loopback health and retries independent
restorations while respecting explicit stops. Initial restoration is immediate.
Unreadable project environments do not stop healthy sites; failed startup
retries are throttled30seconds and duplicate audits suppressed. Process
permissions, private storage, routing, package namespace and installer guard6
remain unchanged. App startup remains interactive-user-logon; SCM acceptance
was not added and pre-sign-in availability must not be claimed.

Native NTFS tests cover six repeated crashes, an unhealthy live process, manual
stop/restart, complete manager/store/router reopen with preserved data and a
broken peer project, runtime ACL recovery, WebSocket restart, cancellation and
control isolation. Latest final validation, release/channel, managed installation
and PC2 delivery are still to record. All test data are synthetic.
