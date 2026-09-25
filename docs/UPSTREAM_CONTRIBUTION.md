# Upstream contribution assessment — 2026-09-09

Candidate: https://github.com/ulule/limiter
Issue: https://github.com/ulule/limiter/issues/94
Related maintainer roadmap: https://github.com/ulule/limiter/issues/92
Inspected default-branch commit: f0ada6c (2024-10-14).

The open issue reports callers retrying before a Redis key expires because the
reported reset time loses fractional seconds. In drivers/store/common/context.go,
GetContextFromState still uses expiration.Unix(), which truncates to seconds.
A maintainer reproduced the issue and explicitly invited PRs in the issue's
2020-05-22 comments. The README contribution section invites forks and bug fixes.

A focused proposal would round future expiration timestamps UP to whole seconds,
with regression cases for fractional and exact-second boundaries, preserving the
public Unix-seconds API. Tests should cover Redis and in-memory consumers of this
shared helper, and distinguish early wakeups from Redis expiry scheduling.

This is a candidate, not a confirmed acceptance opportunity. The checked default
branch has not advanced since October 2024 and multiple PRs are waiting. The
maintainer invitation is old. No upstream PR or message has been submitted. Before
submission, recheck activity and linked PRs, reproduce the failure against upstream,
and review compatibility expectations for the shared helper.

A portfolio repository is not automatically a fork of upstream. A contribution
must be a small fix in an actual upstream fork; link the resulting PR from this
project only after one exists. Do not send this project's entire service upgrade
to an unrelated upstream library.

Other checks: ulule/limiter #263 already has PR #264; redis_rate PerDay already has
PR #102; redis_rate #73 asks for latency benchmarks but has no recent maintainer
commitment. Duplicating those PRs is not a useful contribution strategy.
