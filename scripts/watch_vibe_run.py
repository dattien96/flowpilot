#!/usr/bin/env python3
"""Watchdog for a live vibe flow run (PrivateVault production run).

Polls the local runner over HTTP every --interval seconds (default 15),
tracks progress, and drafts BUG notes under requirements/09-BugFix/todo/
when it sees tool-side anomalies (hang, failed steps, conflicts, runner
outage). It never edits code and never decides approvals / questions /
gates (those stay human). The only state-changing calls it may make are
the safe loop nudges `agent-loop/resume` and `agent-loop/continue`, and
only when ALL of these hold:
  - no human gate is pending (no pendingApproval / pendingQuestion /
    pendingGate),
  - the run is non-terminal with status `running`,
  - no timeline/steps progress for --hang-minutes (default 5),
  - at most one resume + one continue per stall episode.

Usage:
  scripts/watch_vibe_run.py --run <runId>
  scripts/watch_vibe_run.py --project privatevault   # auto-pick latest active run
  scripts/watch_vibe_run.py --list                    # show candidates and exit
  scripts/watch_vibe_run.py --once                    # single poll cycle (smoke test)
  scripts/watch_vibe_run.py --self-test               # unit-check classifiers, no HTTP

Env:
  FLOWPILOT_RUNNER_URL  base URL (default http://127.0.0.1:4317)
"""

import argparse
import datetime
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

TERMINAL = {"completed", "failed", "cancelled"}
HUMAN_GATE_STATUSES = {"waiting_approval", "waiting_user_approval", "waiting_question", "blocked"}
FAIL_STEP_STATUSES = {"FAILED"}
HTTP_TIMEOUT = 10


def utcnow():
    return datetime.datetime.now(datetime.timezone.utc)


def ts():
    return utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")


class Runner:
    def __init__(self, base):
        self.base = base.rstrip("/")

    def _call(self, method, path, body=None):
        url = self.base + path
        data = None
        headers = {"Content-Type": "application/json"}
        if body is not None:
            data = json.dumps(body).encode()
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as resp:
                raw = resp.read().decode("utf-8", "replace")
                return resp.status, (json.loads(raw) if raw.strip() else None)
        except urllib.error.HTTPError as e:
            try:
                payload = json.loads(e.read().decode("utf-8", "replace") or "{}")
            except Exception:
                payload = {"raw": "unreadable"}
            return e.code, {"_http_error": True, "code": payload.get("code"), "msg": payload.get("msg", payload)}
        except Exception as e:  # connection refused, timeout, DNS, ...
            return 0, {"_transport_error": True, "msg": "%s: %s" % (type(e).__name__, e)}

    def get(self, path):
        return self._call("GET", path)

    def post(self, path, body=None):
        return self._call("POST", path, {} if body is None else body)


def summarize_steps(steps):
    """Compact per-status counts + worst offenders for one log line."""
    counts = {}
    worst = []
    for s in steps or []:
        st = str(s.get("status", "?"))
        counts[st] = counts.get(st, 0) + 1
        if st in FAIL_STEP_STATUSES or (s.get("retryCount") or 0) >= 5 or s.get("rejectionNote"):
            worst.append("%s:%s%s%s" % (
                s.get("nodeId") or s.get("stepId") or "?",
                st,
                ("r%d" % s["retryCount"]) if s.get("retryCount") else "",
                ("!" if s.get("rejectionNote") else ""),
            ))
    return counts, worst


def progress_watermark(run, steps, next_seq):
    """Hashable progress fingerprint. Any advance resets the hang timer."""
    step_sig = tuple(sorted(
        (str(s.get("stepId")), str(s.get("status")), int(s.get("retryCount") or 0))
        for s in (steps or [])
    ))
    return (str(run.get("status")), int(next_seq or 0), step_sig)


def next_bug_number(todo_dir):
    mx = 661
    try:
        for name in os.listdir(todo_dir):
            m = re.match(r"BUG-(\d+)", name)
            if m:
                mx = max(mx, int(m.group(1)))
    except OSError:
        pass
    return mx + 1


def draft_bug(todo_dir, title, summary_lines, evidence_lines, tags):
    num = next_bug_number(todo_dir)
    slug = re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")[:60]
    path = os.path.join(todo_dir, "BUG-%d-%s.md" % (num, slug))
    now = ts()
    doc = """# BUG-%d — %s

## Metadata

- Document ID: `BUG-%d`
- Title: `%s`
- Phase: `bugfix`
- Status: `DRAFT (WATCHDOG — not yet triaged, do not fix blindly)`
- Owner: `flowpilot runner`
- Reviewers: `dev`
- Created: `%s`
- Last Updated: `%s`
- Related Documents: `%s`
- Tags: `watchdog, %s`

## AI Quick View

### Summary

%s

### Evidence (watchdog snapshot)

```
%s
```

### Current Ask

- Triage: reproduce from the evidence, confirm tool-side vs app-side, then file a real fix task. The watchdog only observes (plus safe loop nudges); it never repairs code.
""" % (num, title, num, title, now, now, evidence_lines[0] if evidence_lines else "—",
       tags, "\n".join("- " + l for l in summary_lines), "\n".join(evidence_lines))
    with open(path, "w") as f:
        f.write(doc)
    return path


class Watcher:
    def __init__(self, args):
        self.args = args
        self.runner = Runner(args.base)
        self.run_id = args.run
        self.logf = open(args.log, "a")
        self.drafted = set()          # episode signatures already noted
        self.last_seq = 0             # timeline cursor (afterSeq watermark)
        self.last_progress = None     # watermark
        self.last_progress_at = None
        self.acted_resume = False     # safe nudges used in current stall episode
        self.acted_continue = False
        self.consec_errors = 0
        self.outage_noted = False

    def log(self, msg):
        line = "[%s] %s" % (ts(), msg)
        print(line, flush=True)
        self.logf.write(line + "\n")
        self.logf.flush()

    def note(self, signature, title, summary, evidence):
        """Draft a BUG todo once per episode signature. Returns path or None."""
        if signature in self.drafted:
            return None
        path = draft_bug(self.args.todo_dir, title, summary,
                         ["run: %s" % self.run_id] + evidence, "production-watch")
        self.drafted.add(signature)
        self.log("NOTE %s -> %s" % (signature, path))
        return path

    def discover(self):
        st, projects = self.runner.get("/client/projects")
        if st != 200 or not isinstance(projects, list):
            self.log("discover: cannot list projects (http=%s)" % st)
            return None
        want = (self.args.project or "").lower()
        pid = None
        for p in projects:
            if not isinstance(p, dict):
                continue
            name = str(p.get("name", ""))
            if want and want in name.lower():
                pid, pname = p.get("id"), name
                break
        if pid is None:
            self.log("discover: no project matching %r in %d projects" %
                     (self.args.project, len(projects)))
            return None
        st, items = self.runner.get("/client/projects/%s/workflow-runs" % pid)
        if st != 200 or not isinstance(items, list):
            self.log("discover: cannot list runs for project %s (http=%s)" % (pname, st))
            return None
        active = [r for r in items
                  if isinstance(r, dict) and str(r.get("status", "")) not in TERMINAL]
        if not active:
            self.log("discover: project %s has no non-terminal runs" % pname)
            return None
        active.sort(key=lambda r: str(r.get("updatedAt") or r.get("startedAt") or ""))
        pick = active[-1]
        self.log("discover: watching %s status=%s project=%s (%d active)" %
                 (pick.get("runId"), pick.get("status"), pname, len(active)))
        return pick.get("runId")

    def poll_once(self):
        """One 15s cycle. Returns 'done:<status>' when terminal, else 'watch'."""
        r_st, run = self.runner.get("/client/workflow-runs/%s" % self.run_id)
        if r_st != 200 or not isinstance(run, dict) or run.get("status") is None:
            return ("error", "run http=%s %s" % (r_st, json.dumps(run)[:200]))
        s_st, steps_view = self.runner.get(
            "/client/workflow-runs/%s/steps-runtime" % self.run_id)
        steps = steps_view.get("steps") if isinstance(steps_view, dict) else None
        t_st, tl = self.runner.get(
            "/client/workflow-runs/%s/timeline?afterSeq=%d&limit=200" % (self.run_id, self.last_seq))
        if isinstance(tl, dict) and isinstance(tl.get("records"), list):
            for r in tl["records"]:
                try:
                    sq = int(r.get("chatSeq", 0))
                except (TypeError, ValueError):
                    continue
                if sq > self.last_seq:
                    self.last_seq = sq
        next_seq = self.last_seq
        a_st, attn = self.runner.get(
            "/client/workflow-runs/%s/dispatch-attention" % self.run_id)
        items = attn.get("items") if isinstance(attn, dict) else None
        g_st, agents = self.runner.get(
            "/client/workflow-runs/%s/agents" % self.run_id)
        nagents = len(agents) if isinstance(agents, list) else -1

        status = str(run.get("status"))
        counts, worst = summarize_steps(steps)
        wm = progress_watermark(run, steps, next_seq)
        now = time.time()
        if self.last_progress is None or wm != self.last_progress:
            self.last_progress, self.last_progress_at = wm, now
            self.acted_resume = self.acted_continue = False
            progressed = True
        else:
            progressed = False
        idle = now - (self.last_progress_at or now)

        pend_a = (run.get("pendingApproval") or {}).get("approvalId", "")
        pend_q = (run.get("pendingQuestion") or {}).get("questionId", "")
        pend_g = run.get("pendingGate")
        human_gate = bool(pend_a or pend_q or pend_g) or status in HUMAN_GATE_STATUSES

        self.log("poll status=%s nextSeq=%s steps=%s worst=%s agents=%s attn=%s idle=%ds%s" % (
            status, next_seq, counts, worst or "-", nagents,
            len(items) if isinstance(items, list) else "?",
            int(idle), " PROGRESS" if progressed else ""))

        ev = ["status=%s nextSeq=%s idle=%ds" % (status, next_seq, int(idle)),
              "steps=%s" % json.dumps(counts),
              "pendingApproval=%s pendingQuestion=%s pendingGate=%s" %
              (bool(pend_a), bool(pend_q), bool(pend_g))]

        # 1. Terminal states close the watch.
        if status in TERMINAL:
            if status == "completed":
                self.log("DONE run completed")
                return ("done", "completed")
            self.note("terminal:%s" % status, "Watch: run terminal-%s" % status,
                      ["Run reached terminal status %r under watchdog." % status,
                       "Needs triage whether app-side or tool-side caused it."],
                      ev + ["worst=%s" % (worst or "-")])
            return ("done", status)

        # 2. Failed steps / rejection notes / hot retries -> NOTE (never auto-fix).
        for s in (steps or []):
            sid = s.get("nodeId") or s.get("stepId") or "?"
            if str(s.get("status")) in FAIL_STEP_STATUSES:
                self.note("step-failed:%s:%s" % (sid, s.get("status")),
                          "Watch: step %s FAILED" % sid,
                          ["Step %r terminal-failed; watchdog does not retry (fix needs triage)." % sid],
                          ev + ["rejectionNote=%s" % (s.get("rejectionNote") or "-"),
                                "retryCount=%s" % (s.get("retryCount") or 0)])
            elif (s.get("retryCount") or 0) >= 5:
                self.note("step-retry:%s:%s" % (sid, s.get("retryCount")),
                          "Watch: step %s hot-retrying" % sid,
                          ["retryCount=%s without terminal state; possible retry-loop bug." % s.get("retryCount")],
                          ev)
            elif s.get("rejectionNote") and str(s.get("status")) != "DONE":
                self.note("step-reject:%s" % sid, "Watch: step %s has rejectionNote" % sid,
                          ["rejectionNote=%r" % s.get("rejectionNote")], ev)

        # 3. Dispatch-attention items (conflicts / arbitration) -> NOTE.
        if isinstance(items, list) and items:
            kinds = sorted({str(i.get("kind") or i.get("type") or "?") for i in items
                            if isinstance(i, dict)})
            self.note("attention:%s" % (",".join(kinds) or "items"),
                      "Watch: dispatch-attention items present",
                      ["%d attention item(s) kinds=%s; possible conflict needing triage." %
                       (len(items), kinds)],
                      ev + [json.dumps(items)[:800]])

        # 4. Worktree red flags (best-effort string scan).
        wt = run.get("worktree")
        if wt:
            blob = json.dumps(wt).lower()
            if any(k in blob for k in ("conflict", "lost", "failed", "error")):
                self.note("worktree:%s" % status, "Watch: worktree red flag",
                          ["worktree projection contains a red-flag token."], ev + [blob[:400]])

        # 5. Hang: no progress beyond threshold.
        if idle >= self.args.hang_minutes * 60:
            if human_gate:
                # Waiting on a human is not a tool hang — but a gate that
                # never clears is a known bug shape, so note once at 2x.
                if idle >= 2 * self.args.hang_minutes * 60:
                    self.note("gate-stall:%s" % status,
                              "Watch: human gate never clears",
                              ["status=%r with a pending human gate and no progress for %ds; "
                               "possible stale-gate tool bug (or just an absent human)." % (status, int(idle))],
                              ev)
                else:
                    self.log("gate-wait status=%s idle=%ds (human, no draft yet)" % (status, int(idle)))
            else:
                self.note("hang:%s" % status, "Watch: no progress (hang)",
                          ["No timeline/steps movement for %ds while status=%r." % (int(idle), status)],
                          ev)
                if not self.args.no_act and status == "running":
                    if not self.acted_resume:
                        c, _ = self.runner.post(
                            "/client/workflow-runs/%s/agent-loop/resume" % self.run_id, {})
                        self.acted_resume = True
                        self.log("ACT resume http=%s (safe nudge #1)" % c)
                    elif not self.acted_continue:
                        c, _ = self.runner.post(
                            "/client/workflow-runs/%s/agent-loop/continue" % self.run_id, {})
                        self.acted_continue = True
                        self.log("ACT continue http=%s (safe nudge #2)" % c)
        return ("watch", status)

    def run(self):
        if not self.run_id:
            self.run_id = self.discover()
            if not self.run_id:
                self.log("no run to watch; exit 3")
                return 3
        self.log("watch start run=%s base=%s interval=%ss hang=%sm" %
                 (self.run_id, self.args.base, self.args.interval, self.args.hang_minutes))
        if self.args.once:
            outcome, detail = self.guarded_cycle()
            self.log("once -> %s %s" % (outcome, detail))
            return 0 if outcome == "watch" else (0 if detail == "completed" else 2)
        while True:
            outcome, detail = self.guarded_cycle()
            if outcome == "done":
                self.log("watch end: %s" % detail)
                return 0 if detail == "completed" else 2
            time.sleep(self.args.interval)

    def guarded_cycle(self):
        try:
            outcome, detail = self.poll_once()
            self.consec_errors = 0
            self.outage_noted = False
            return outcome, detail
        except Exception as e:  # transportanez failure: never die, keep watching
            self.consec_errors += 1
            self.log("poll-error %s (%d consecutive)" % (e, self.consec_errors))
            if self.consec_errors >= 4 and not self.outage_noted:
                self.outage_noted = True
                self.note("outage", "Watch: runner unreachable",
                          ["%d consecutive poll failures: %s." % (self.consec_errors, e),
                           "Flow may still progress; watchdog keeps retrying."],
                          ["base=%s" % self.args.base])
            return ("watch", "error")


# ---- self-test: pure classifiers on fixtures, no HTTP ----

def _self_test():
    counts, worst = summarize_steps([
        {"stepId": "a", "status": "DONE"},
        {"nodeId": "coder", "status": "RUNNING"},
        {"nodeId": "bad", "status": "FAILED", "retryCount": 2, "rejectionNote": "x"},
        {"nodeId": "hot", "status": "RUNNING", "retryCount": 7},
    ])
    assert counts == {"DONE": 1, "RUNNING": 2, "FAILED": 1}, counts
    assert worst[0].startswith("bad:FAILED"), worst
    assert worst[1] == "hot:RUNNINGr7", worst
    w1 = progress_watermark({"status": "running"}, [{"stepId": "a", "status": "DONE"}], 41)
    w2 = progress_watermark({"status": "running"}, [{"stepId": "a", "status": "DONE"}], 42)
    assert w1 != w2
    assert next_bug_number("/nonexistent-dir-xyz") == 662
    print("self-test ok")


def main(argv=None):
    ap = argparse.ArgumentParser(description="Watchdog for a live vibe flow run.")
    ap.add_argument("--base", default=os.environ.get("FLOWPILOT_RUNNER_URL", "http://127.0.0.1:4317"))
    ap.add_argument("--run", default="")
    ap.add_argument("--project", default="privatevault")
    ap.add_argument("--interval", type=int, default=15)
    ap.add_argument("--hang-minutes", type=int, default=5)
    ap.add_argument("--todo-dir", default=os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                                       "..", "requirements", "09-BugFix", "todo"))
    ap.add_argument("--log", default=os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                                  "privatevault-watch.log"))
    ap.add_argument("--once", action="store_true")
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--no-act", action="store_true",
                    help="observe + note only; disable resume/continue nudges")
    ap.add_argument("--self-test", action="store_true")
    args = ap.parse_args(argv)
    if args.self_test:
        _self_test()
        return 0
    w = Watcher(args)
    if args.list:
        rid = w.discover()
        print("pick: %s" % rid)
        return 0 if rid else 3
    return w.run()


if __name__ == "__main__":
    sys.exit(main())
