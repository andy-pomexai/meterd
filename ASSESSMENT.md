# Backend take-home

Thanks for taking the time. This exercise is built around a small Go service,
`meterd`. It has three parts that share one codebase and one data set. Plan for
about **two hours** in total.

The company and the service in this exercise are fictional. The repository you
got the link to is public and read-only. You work in a private copy of your
own (see "Setup") and hand in everything there.

## Ground rules

- **Use the tools you normally use**, including AI assistants and coding
  agents. We do too.
- **A correct write-up is the baseline, not the goal.** We ran this exercise
  through a coding agent with a one-line prompt and nobody checking its work.
  It does well on the written deliverables. So what we evaluate is what *you*
  add on top: what you check, what you decide, what you overrule, and what you
  choose to leave out.
- **Show us how you worked.** Export your conversations with every AI tool you
  used and include them (see "Submitting"). We read your side of those
  conversations closely; it is the best evidence we have of how you think.
  Heavy use of AI is fine. Handing over this brief and submitting whatever
  comes back is not what we are hiring for.
- **Time-box it.** Two hours is not enough to do everything perfectly, and that
  is deliberate. Decide what matters most and tell us what you left out and
  why. The page limits below are part of the exercise. Please do not spend an
  evening on this.
- **We will talk about it.** If we move forward, the next step is a 30-minute
  call in which you walk us through your submission and extend it live. Submit
  only what you understand and can defend.
- **There is nobody to ask.** Where something is ambiguous, make a reasonable
  assumption and write it down.

## Setup

Make a private copy of the public repository, with its history and both of its
branches. Please do not fork it: a fork of a public repository is public.

1. On GitHub, create a new, empty, **private** repository under your own
   account (no README, no .gitignore, no licence).
2. Copy the public repository into it:

   ```
   git clone --bare <URL of the public repository> meterd-source.git
   git -C meterd-source.git push --mirror <URL of your private repository>
   rm -rf meterd-source.git
   ```

3. Clone your copy, check that it works, and create the branch that all of
   your work goes on:

   ```
   git clone <URL of your private repository>
   cd <its folder>
   go test ./...        # passes; Go 1.26+, the first run downloads one dependency
   git branch -r        # lists origin/main and origin/feat/cached-token-pricing
   git checkout -b fix/meter-142
   ```

From here on you only ever push to your private copy. Everything you write for
this exercise, code and documents alike, goes on `fix/meter-142`.

Start with `README.md`, `docs/events.md` and `docs/billing-rules.md`. They are
short and everything below depends on them.

## Part 1: review a pull request (about 25 minutes)

In the public repository a teammate has opened pull request #1, "Cached token
pricing", from the branch `feat/cached-token-pricing`. Read it there. Its
description is also in `assessment/PR-1.md`, and you can read the change
locally:

```
git diff origin/main...origin/feat/cached-token-pricing
```

The conversation on that pull request is locked. Do not post your review there
or anywhere else in public. Write the review you would post in
`submission/REVIEW.md` (one page):

- your verdict: approve, approve with comments, or request changes;
- your findings, most important first. For each one: what is wrong, why it
  matters in production, and what you want the author to do. Where a finding
  belongs to a particular place, name the file and line.

We care far more about the few things that matter than about a long list. You
do not need to fix the PR.

## Part 2: fix a production issue (about 45 minutes)

`assessment/METER-142.md` is a ticket from Platform Support. Work on your
`fix/meter-142` branch, which starts from `main`, not from the PR branch. When
you push it, open a pull request from it into `main` in your private copy. Do
not merge it.

- Find out what is going on, fix it, and prove the fix with tests.
- Write up, in `submission/NOTES.md` (two pages including "Your part" below):
  the root cause or causes; what you changed and why; your answer to the
  tenant's question, with the numbers behind it; what should happen to data
  that is already wrong; and anything you found but chose not to change.
- Keep the description of your pull request to a few lines. The write-up
  belongs in `submission/NOTES.md`.

We run our own tests against your branch, through the HTTP API and
`internal/server`. Keep `server.New`, `Config.DBPath`, `Config.FlushInterval`,
`Handler` and `Close` working.

## Part 3: reconcile an invoice (about 40 minutes)

`data/upstream_invoice_2026-10.csv` is the October invoice from the upstream
model provider. Finance asks: **does it match what we metered?**

Reconcile it against `data/snapshot.db` and write `submission/RECON.md` (two
pages):

- every difference you find: how big it is (in tokens, and in USD where you
  can), what causes it, how sure you are, and what should happen next;
- put whatever you used to get there (scripts, queries) in `submission/recon/`
  so that we can run it again.

`data/README.md` describes the files.

## Your part (about 10 minutes)

At the top of `submission/NOTES.md`:

- **Where you made the difference: at most three moments.** Places where the
  result is different because of you: something you decided, corrected,
  refused, or insisted on checking. For each one, say what happened, what would
  have been submitted otherwise, and where to find it in your AI logs (the file
  and a phrase we can search for). We check these against the logs. If the
  honest answer is "nothing", say so. If you used no AI, give the three
  decisions of your own that mattered most.
- The assumptions you made.
- What you left out, and what you would do next with another day.

## Submitting

Everything is handed in through your private copy:

1. Your pull request from `fix/meter-142` into `main`, left open. It carries
   your commits (several small commits are better than one large one) and the
   whole `submission/` folder: `REVIEW.md`, `NOTES.md`, `RECON.md`, `recon/`
   and `ai/`.
2. In `submission/ai/`, your AI conversations, complete and unedited, as text
   or Markdown: your prompts and the tool's answers. Most tools can export a
   session (in Claude Code: `/export`; in Cursor and ChatGPT: export or copy
   the chat). Next to them, add `submission/ai/PROMPTS.md` with only your own
   messages, in order and word for word, so that we can read your side in one
   go. Remove secrets if there are any, and change nothing else.
3. Access for us: in your repository, under Settings, then Collaborators, add
   the GitHub account named in our email.

Before you finish, check on GitHub that your pull request contains your last
commit. Then reply to our email with the address of your repository. We take a
copy of it when your reply arrives, or at the deadline in that email if that
comes first. Later changes do not count.

Please keep your copy private, and do not publish the exercise or your
solution anywhere else.

## What we look at

- the baseline: whether what you ship is correct, and how it was verified;
- what you added to what the tools gave you;
- how clearly you separate what you know from what you assume;
- restraint and prioritisation: small changes, each with a reason, inside the
  time and page limits.
