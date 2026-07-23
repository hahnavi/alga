use alga_agent_sdk::InvestigationCommand;
use serde_json::{json, Value};

/// Verifies the wire shape of every command variant against what the backend's
/// `InvTool` struct expects. Guards the incident_number-vs-incident_id
/// regression.
fn round_trip(cmd: InvestigationCommand) -> Value {
    let raw = serde_json::to_value(&cmd).expect("serialize command");
    // Commands are sent nested under a "command" object by the client; mirror
    // that so the test reflects reality.
    json!({ "command": raw })
}

#[test]
fn resolve_alert() {
    let v = round_trip(InvestigationCommand::ResolveAlert {
        fingerprint: "fp123".into(),
        root_cause: None,
        resolution: None,
    });
    assert_eq!(v["command"]["op"], "resolve_alert");
    assert_eq!(v["command"]["fingerprint"], "fp123");
}

#[test]
fn set_incident_priority_uses_incident_number() {
    let v = round_trip(InvestigationCommand::SetIncidentPriority {
        incident_number: 42,
        priority: "high".into(),
    });
    assert_eq!(v["command"]["op"], "set_incident_priority");
    assert_eq!(v["command"]["incident_number"], 42);
    assert!(
        v["command"].get("incident_id").is_none(),
        "must not emit incident_id"
    );
}

#[test]
fn incident_commands_never_emit_incident_id() {
    let cmds = vec![
        round_trip(InvestigationCommand::TriggerEscalation { incident_number: 7 }),
        round_trip(InvestigationCommand::MitigateIncident {
            incident_number: 7,
            reason: None,
        }),
        round_trip(InvestigationCommand::ResolveIncident {
            incident_number: 7,
            reason: None,
        }),
        round_trip(InvestigationCommand::BeginTriage { incident_number: 7 }),
        round_trip(InvestigationCommand::PromoteIncident { incident_number: 7 }),
    ];
    for c in cmds {
        assert!(
            c["command"].get("incident_id").is_none(),
            "incident command serialized banned field incident_id: {c}"
        );
        // incident_number must be a JSON number, not a string.
        assert!(
            c["command"]["incident_number"].is_number(),
            "incident_number must be a number: {c}"
        );
    }
}

#[test]
fn post_handoff() {
    let v = round_trip(InvestigationCommand::PostHandoff {
        incident_number: 11,
        message: "handing off".into(),
        audience: "commander".into(),
        urgency: "info".into(),
    });
    assert_eq!(v["command"]["op"], "post_handoff");
    assert_eq!(v["command"]["incident_number"], 11);
    assert_eq!(v["command"]["message"], "handing off");
    assert_eq!(v["command"]["audience"], "commander");
    assert_eq!(v["command"]["urgency"], "info");
}

#[test]
fn publish_status_update() {
    let v = round_trip(InvestigationCommand::PublishStatusUpdate {
        incident_number: 11,
        message: "root cause found".into(),
        status_level: "identified".into(),
        impact_assessment: None,
        actions_taken: None,
        eta: None,
        source_coordination_message_id: None,
    });
    assert_eq!(v["command"]["op"], "publish_status_update");
    assert_eq!(v["command"]["status_level"], "identified");
}

#[test]
fn set_incident_resolution_docs() {
    let v = round_trip(InvestigationCommand::SetIncidentResolutionDocs {
        incident_number: 11,
        summary: Some("s".into()),
        impact_assessment: None,
        actions_taken: None,
        root_cause: Some("rc".into()),
        resolution: Some("res".into()),
    });
    assert_eq!(v["command"]["op"], "set_incident_resolution_docs");
    assert_eq!(v["command"]["summary"], "s");
    assert_eq!(v["command"]["root_cause"], "rc");
    assert_eq!(v["command"]["resolution"], "res");
}
