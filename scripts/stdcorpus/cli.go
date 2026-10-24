package main

import "strings"

// std/cli's Parsed.duration_opt, which no corpus reached: std.md 14.4 says
// an accessor's _opt form is None when the flag was not given on the command
// line or in its variable, even when the flag declares a default. The value
// text is Duration.parse syntax (Go's time.ParseDuration), and a bad value
// is a Usage error with exit status 2. The flag's env variable is read
// through the test's fake Env, so no Go twin is needed: the rules are the
// spec's, and the durations are Go's.
func cliCorpus() string {
	var o strings.Builder
	o.WriteString(header("std/cli's Parsed.duration_opt: Some only when the flag was given on the command line or in its variable, the default left to duration(), and a malformed value a Usage error with exit status 2 (std.md 14.4)"))
	o.WriteString("use std/cli\n")
	o.WriteString(cliTests)
	return o.String()
}

const cliTests = `
fn dur_text(d: Duration?) -> Str {
  match d {
    Some(x) => "${x}"
    None => "none"
  }
}

test "duration_opt is Some only when the flag was given" {
  let cmd = cli.command("app", "h", flags: [cli.duration_flag("wait", "how long")])
  let given = cli.parse(cmd, ["--wait", "1m30s"], os.env)?
  expect(dur_text(given.duration_opt("wait"))) == "1m30s"
  expect("${given.duration("wait")}") == "1m30s"
  expect(given.is_set("wait")) == true
  let absent = cli.parse(cmd, [], os.env)?
  expect(dur_text(absent.duration_opt("wait"))) == "none"
  expect("${absent.duration("wait")}") == "0s"
  expect(absent.is_set("wait")) == false
}

test "a valued variable counts, a default does not" {
  let cmd = cli.command("app", "h", flags: [
    cli.duration_flag("wait", "how long", env: "CORPUS_WAIT", default: 30s),
  ])
  let dflt = cli.parse(cmd, [], os.env)?
  expect(dur_text(dflt.duration_opt("wait"))) == "none"
  expect("${dflt.duration("wait")}") == "30s"
  os.env.set("CORPUS_WAIT", "250ms")
  let from_env = cli.parse(cmd, [], os.env)?
  expect(dur_text(from_env.duration_opt("wait"))) == "250ms"
  expect(from_env.is_set("wait")) == true
}

test "a malformed duration is a Usage error with exit status 2" {
  let cmd = cli.command("app", "h", flags: [cli.duration_flag("wait", "how long")])
  expect(match cli.parse(cmd, ["--wait", "nope"], os.env) {
    Ok(_) => "ok"
    Err(Usage(_, _)) => "usage"
    Err(_) => "other"
  }) == "usage"
  expect(match cli.parse(cmd, ["--wait", "nope"], os.env) {
    Ok(_) => 0
    Err(e) => e.exit_code()
  }) == 2
}

test "FakeEnv.set_args is what env.args gives back" {
  os.env.set_args(["app", "--wait", "2s"])
  expect(os.env.args()) == ["app", "--wait", "2s"]
}
`
