use alga_agent_sdk::commands::*;
use alga_agent_sdk::InvestigationCommand;
use serde_json::{json, Value};

fn round_trip(cmd: InvestigationCommand) -> Value {
    let raw = serde_json::to_value(&cmd).expect("serialize command");
    json!({ "command": raw })
}

fn assert_no_incident_id(v: &Value) {
    assert!(
        v["command"].get("incident_id").is_none(),
        "command serialized banned field incident_id: {v}"
    );
}

#[test]
fn test_resolve_alert() {
    let v = round_trip(resolve_alert("fp123"));
    assert_eq!(v["command"]["op"], "resolve_alert");
    assert_eq!(v["command"]["fingerprint"], "fp123");
    assert_no_incident_id(&v);
}

#[test]
fn test_reopen_alert() {
    let v = round_trip(reopen_alert("fp456"));
    assert_eq!(v["command"]["op"], "reopen_alert");
    assert_eq!(v["command"]["fingerprint"], "fp456");
}

#[test]
fn test_set_outcome() {
    let v = round_trip(set_outcome(Some("db pool exhausted"), None));
    assert_eq!(v["command"]["op"], "set_outcome");
    assert_eq!(v["command"]["root_cause"], "db pool exhausted");
    assert!(
        v["command"].get("resolution").is_none(),
        "None resolution must be omitted"
    );
}

#[test]
fn test_set_outcome_empty_strings_preserved() {
    let v = round_trip(set_outcome(Some(""), Some("")));
    assert_eq!(v["command"]["root_cause"], "");
    assert_eq!(v["command"]["resolution"], "");
}

#[test]
fn test_cancel_and_pause_investigation() {
    let v = round_trip(cancel_investigation("dup of #42"));
    assert_eq!(v["command"]["op"], "cancel_investigation");
    assert_eq!(v["command"]["reason"], "dup of #42");

    let v2 = round_trip(pause_investigation(""));
    assert_eq!(v2["command"]["op"], "pause_investigation");
    assert!(
        v2["command"].get("reason").is_none(),
        "empty reason must be omitted"
    );
}

#[test]
fn test_triage_feedback() {
    let v = round_trip(triage_feedback("tr-1", true, "correct", "high", "looks right"));
    assert_eq!(v["command"]["op"], "triage_feedback");
    assert_eq!(v["command"]["triage_result_id"], "tr-1");
    assert_eq!(v["command"]["agreed"], true);
    assert_eq!(v["command"]["correct_decision"], "correct");
    assert_eq!(v["command"]["correct_severity"], "high");
    assert_eq!(v["command"]["note"], "looks right");
}

#[test]
fn test_triage_feedback_disagreed_omits_bool() {
    let v = round_trip(triage_feedback("tr-2", false, "", "", ""));
    assert_eq!(v["command"]["op"], "triage_feedback");
    assert!(
        v["command"].get("agreed").is_none(),
        "agreed=false must be omitted (Go omitempty)"
    );
}

#[test]
fn test_assign_investigation() {
    let v = round_trip(assign_investigation("agent-007"));
    assert_eq!(v["command"]["op"], "assign_investigation");
    assert_eq!(v["command"]["target_agent_id"], "agent-007");
}

#[test]
fn test_promote_to_incident() {
    let v = round_trip(promote_to_incident("DB outage", "critical", "P1"));
    assert_eq!(v["command"]["op"], "promote_to_incident");
    assert_eq!(v["command"]["title"], "DB outage");
    assert_eq!(v["command"]["severity"], "critical");
    assert_eq!(v["command"]["priority"], "P1");
}

#[test]
fn test_set_incident_priority_uses_incident_number() {
    let v = round_trip(set_incident_priority(42, "high"));
    assert_eq!(v["command"]["op"], "set_incident_priority");
    assert_eq!(v["command"]["incident_number"], 42);
    assert!(
        v["command"]["incident_number"].is_number(),
        "incident_number must be a JSON number"
    );
    assert_no_incident_id(&v);
}

#[test]
fn test_set_incident_severity() {
    let v = round_trip(set_incident_severity(7, "sev2"));
    assert_eq!(v["command"]["op"], "set_incident_severity");
    assert_eq!(v["command"]["incident_number"], 7);
    assert_eq!(v["command"]["severity"], "sev2");
    assert_no_incident_id(&v);
}

#[test]
fn test_incident_commands_never_emit_incident_id() {
    let cmds = vec![
        round_trip(trigger_escalation(7)),
        round_trip(mitigate_incident(7, "patched")),
        round_trip(resolve_incident(7, "resolved")),
        round_trip(begin_triage(7)),
        round_trip(promote_incident(7)),
    ];
    for c in cmds {
        assert_no_incident_id(&c);
        assert!(
            c["command"]["incident_number"].is_number(),
            "incident_number must be a number: {c}"
        );
    }
}

#[test]
fn test_assign_incident_role_to_user() {
    let v = round_trip(assign_incident_role_to_user(3, "commander", "u-1", "all services"));
    assert_eq!(v["command"]["op"], "assign_incident_role");
    assert_eq!(v["command"]["incident_number"], 3);
    assert_eq!(v["command"]["role_type"], "commander");
    assert_eq!(v["command"]["user_id"], "u-1");
    assert_eq!(v["command"]["scope_description"], "all services");
    assert!(
        v["command"].get("agent_token_id").is_none(),
        "user variant must not set agent_token_id"
    );
    assert_no_incident_id(&v);
}

#[test]
fn test_assign_incident_role_to_agent() {
    let v = round_trip(assign_incident_role_to_agent(3, "responder", "tok-9", ""));
    assert_eq!(v["command"]["op"], "assign_incident_role");
    assert_eq!(v["command"]["agent_token_id"], "tok-9");
    assert!(
        v["command"].get("scope_description").is_none(),
        "empty scope_description must be omitted"
    );
    assert!(
        v["command"].get("user_id").is_none(),
        "agent variant must not set user_id"
    );
}

#[test]
fn test_post_handoff() {
    let v = round_trip(post_handoff(11, "handing off", "commander", "info"));
    assert_eq!(v["command"]["op"], "post_handoff");
    assert_eq!(v["command"]["incident_number"], 11);
    assert_eq!(v["command"]["message"], "handing off");
    assert_eq!(v["command"]["audience"], "commander");
    assert_eq!(v["command"]["urgency"], "info");
    assert_no_incident_id(&v);
}

#[test]
fn test_publish_status_update() {
    let v = round_trip(publish_status_update(11, "root cause found", "identified"));
    assert_eq!(v["command"]["op"], "publish_status_update");
    assert_eq!(v["command"]["incident_number"], 11);
    assert_eq!(v["command"]["message"], "root cause found");
    assert_eq!(v["command"]["status_level"], "identified");
    assert_no_incident_id(&v);
}

#[test]
fn test_set_incident_resolution_docs() {
    let v = round_trip(set_incident_resolution_docs(11, "s", "ia", "at", "rc", "res"));
    assert_eq!(v["command"]["op"], "set_incident_resolution_docs");
    assert_eq!(v["command"]["incident_number"], 11);
    assert_eq!(v["command"]["summary"], "s");
    assert_eq!(v["command"]["impact_assessment"], "ia");
    assert_eq!(v["command"]["actions_taken"], "at");
    assert_eq!(v["command"]["root_cause"], "rc");
    assert_eq!(v["command"]["resolution"], "res");
    assert_no_incident_id(&v);
}

#[test]
fn test_set_incident_resolution_docs_omits_empty_pointers() {
    let v = round_trip(set_incident_resolution_docs(11, "s", "", "", "", ""));
    assert_eq!(v["command"]["summary"], "s");
    assert!(
        v["command"].get("root_cause").is_none(),
        "empty root_cause must be omitted"
    );
    assert!(
        v["command"].get("resolution").is_none(),
        "empty resolution must be omitted"
    );
}

#[test]
fn test_request_status_update_not_present() {
    let v = round_trip(resolve_alert("fp-check"));
    assert!(
        v["command"].get("request_status_update").is_none(),
        "request_status_update was removed from the SDK"
    );
}
