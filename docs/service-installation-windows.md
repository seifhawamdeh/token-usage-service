# Windows installation

Windows has no direct systemd/launchd equivalent for a one-shot, per-user CLI.
This guide uses **Task Scheduler**, either through the GUI or through
PowerShell's `ScheduledTask` cmdlets. Both produce the same task; pick
whichever fits your workflow. All commands run from PowerShell.

For interactive setup from the repository root, run:

```powershell
.\scripts\install\windows.ps1
```

The sections below document the equivalent manual setup.

## 1. Prerequisites

Check whether Go is already installed:

```powershell
go version
```

If that fails with `The term 'go' is not recognized...`, install it with
winget and reopen the shell so the updated `PATH` takes effect:

```powershell
winget install GoLang.Go
```

Close and reopen PowerShell, then confirm:

```powershell
go version
```

You also need a reachable PostgreSQL database. `validate-config` in step 3
below will fail clearly if it cannot connect.

## 2. Get the repository and build

```powershell
git clone https://github.com/seifhawamdeh/token-usage-service.git C:\Services\token-usage-service
Set-Location C:\Services\token-usage-service
Copy-Item .env.example .env
New-Item -ItemType Directory -Force .\bin | Out-Null
go build -o C:\Services\token-usage-service\bin\token-usage-ingest.exe .\cmd\ingest
go build -o C:\Services\token-usage-service\bin\token-usage-loadworkstyle.exe .\cmd\loadworkstyle
go build -o C:\Services\token-usage-service\bin\token-usage-dashboard.exe .\cmd\dashboard
```

The first build downloads module dependencies and can take a minute or two
longer than subsequent builds.

`token-usage-loadworkstyle.exe` loads raw slash-command and prompt history (no
token/cost data) into a staging table for later analysis. It shares
`DATABASE_URL` and `HOST_ID` from `.env`; parsing into structured fields is
not implemented yet.

## 3. Configure, validate, and run once

Edit `.env` and set `DATABASE_URL`, `HOST_ID`, and `ENABLED_VENDORS`, then:

```powershell
.\bin\token-usage-ingest.exe validate-config
.\bin\token-usage-ingest.exe migrate
.\bin\token-usage-ingest.exe ingest
.\bin\token-usage-loadworkstyle.exe `
  claude_history=$env:USERPROFILE\.claude\history.jsonl `
  claude_caveman_history=$env:USERPROFILE\.claude\.caveman-history.jsonl `
  codex_history=$env:USERPROFILE\.codex\history.jsonl `
  codex_session_index=$env:USERPROFILE\.codex\session_index.jsonl
```

The manual ingest should finish with `errors=0` before installing the
scheduled task.

Windows locations for provider databases vary by provider version. Set
explicit path overrides in `.env` when a Linux-style default does not exist,
especially `OPENCODE_DB_PATH` and `CURSOR_STATE_DB`.

## 4. Create the scheduled task

Pick one of the two paths below. Both result in a task with two actions —
`token-usage-ingest.exe ingest` then `token-usage-loadworkstyle.exe` — run in
order every 30 minutes, only while the user is logged on (needed to read that
user's provider data), without starting overlapping runs.

### Path A: Task Scheduler GUI

1. Select **Create Task**, name it `Token Usage Ingest`, and choose **Run only
   when user is logged on** so the task can read that user's provider data.
2. Add a daily trigger, set it to repeat every **30 minutes** for a duration
   of **Indefinitely**.
3. Add action **Start a program**. Program:
   `C:\Services\token-usage-service\bin\token-usage-ingest.exe`; arguments:
   `ingest`; start in: `C:\Services\token-usage-service`.
4. Add a second action, same **Start a program** type, that runs after the
   first: Program: `C:\Services\token-usage-service\bin\token-usage-loadworkstyle.exe`;
   arguments:
   `claude_history=%USERPROFILE%\.claude\history.jsonl claude_caveman_history=%USERPROFILE%\.claude\.caveman-history.jsonl codex_history=%USERPROFILE%\.codex\history.jsonl codex_session_index=%USERPROFILE%\.codex\session_index.jsonl`;
   start in: `C:\Services\token-usage-service`. Task Scheduler runs multiple
   actions in the order listed.
5. In **Settings**, enable **Run task as soon as possible after a scheduled
   start is missed** and disable parallel starts by choosing **Do not start a
   new instance**.
6. Run the task once. Confirm **Last Run Result** is `0x0`; inspect the Windows
   Task Scheduler operational log if it is not.

The **Start in** value is required: it lets the executable load the repository
`.env` and resolve relative paths consistently.

### Path B: PowerShell script

Equivalent to path A, scripted. Run from an elevated PowerShell prompt:

```powershell
$ingestAction = New-ScheduledTaskAction -Execute 'C:\Services\token-usage-service\bin\token-usage-ingest.exe' `
             -Argument 'ingest' -WorkingDirectory 'C:\Services\token-usage-service'
$workstyleArgs = 'claude_history=' + $env:USERPROFILE + '\.claude\history.jsonl ' +
                 'claude_caveman_history=' + $env:USERPROFILE + '\.claude\.caveman-history.jsonl ' +
                 'codex_history=' + $env:USERPROFILE + '\.codex\history.jsonl ' +
                 'codex_session_index=' + $env:USERPROFILE + '\.codex\session_index.jsonl'
$workstyleAction = New-ScheduledTaskAction -Execute 'C:\Services\token-usage-service\bin\token-usage-loadworkstyle.exe' `
             -Argument $workstyleArgs -WorkingDirectory 'C:\Services\token-usage-service'
$trigger = New-ScheduledTaskTrigger -Daily -At 12am
$rep = (New-ScheduledTaskTrigger -Once -At (Get-Date) `
          -RepetitionInterval (New-TimeSpan -Minutes 30) `
          -RepetitionDuration (New-TimeSpan -Days 3650)).Repetition
$trigger.Repetition = $rep
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew
Register-ScheduledTask -TaskName 'Token Usage Ingest' -Action $ingestAction, $workstyleAction -Trigger $trigger `
  -Settings $settings -RunLevel Limited -Description 'Collects local AI usage token data every 30 minutes'
Start-ScheduledTask -TaskName 'Token Usage Ingest'
```

> **Gotcha — `RepetitionDuration` cannot be `[TimeSpan]::MaxValue`.**
> It is tempting to express "repeat indefinitely" with
> `-RepetitionDuration ([TimeSpan]::MaxValue)`. `Register-ScheduledTask` rejects
> it with:
> ```
> Register-ScheduledTask : The task XML contains a value which is incorrectly
> formatted or out of range. (10,42):Duration:P99999999DT23H59M59S
> ```
> `MaxValue` (~10,675,199 days) overflows Task Scheduler's XML `duration` field.
> Use a large but bounded value instead — `New-TimeSpan -Days 3650` (10 years)
> is effectively indefinite for this purpose and is what the snippet above
> uses.

## 5. Verify

GUI: open Task Scheduler, select `Token Usage Ingest`, and check **Last Run
Result** is `0x0`.

PowerShell equivalent:

```powershell
Get-ScheduledTaskInfo -TaskName 'Token Usage Ingest' |
  Select-Object LastRunTime, LastTaskResult, NextRunTime
```

`LastTaskResult` of `0` means success. Anything else — inspect the Windows
Task Scheduler operational log (Event Viewer → Applications and Services Logs
→ Microsoft → Windows → TaskScheduler → Operational).

## Windows dashboard

Task Scheduler can also start the dashboard at logon:

1. Create task `Token Usage Dashboard` with an **At log on** trigger.
2. Set program to `token-usage-dashboard.exe` and **Start in** to the repository.
3. Disable the setting that stops the task after a time limit.
4. Run it and open `http://127.0.0.1:8080`.

Or via PowerShell:

```powershell
$action  = New-ScheduledTaskAction -Execute 'C:\Services\token-usage-service\bin\token-usage-dashboard.exe' `
             -WorkingDirectory 'C:\Services\token-usage-service'
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName 'Token Usage Dashboard' -Action $action -Trigger $trigger `
  -Settings $settings -RunLevel Limited
```

`ExecutionTimeLimit` of zero means "no limit" (unlike `RepetitionDuration`,
`[TimeSpan]::Zero` is valid here since it maps to a different XML field with
different bounds).

For an always-on machine where no user logs in, use a dedicated Windows service
account that owns or can read the provider files. Do not run under
`LocalSystem`: its home directory does not contain the user's transcripts.

## Upgrades

```powershell
git pull --ff-only
go build -o C:\Services\token-usage-service\bin\token-usage-ingest.exe .\cmd\ingest
go build -o C:\Services\token-usage-service\bin\token-usage-loadworkstyle.exe .\cmd\loadworkstyle
go build -o C:\Services\token-usage-service\bin\token-usage-dashboard.exe .\cmd\dashboard
.\bin\token-usage-ingest.exe migrate
```

Stop the `Token Usage Dashboard` task before replacing its executable
(`Stop-ScheduledTask -TaskName 'Token Usage Dashboard'`), then start it again
after the rebuild.

## Removal

```powershell
Unregister-ScheduledTask -TaskName 'Token Usage Ingest' -Confirm:$false
Unregister-ScheduledTask -TaskName 'Token Usage Dashboard' -Confirm:$false
```

Removing the scheduled task does not remove PostgreSQL data or the cloned
repository; delete those separately if needed.
