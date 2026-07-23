import asyncio
import importlib.util
import json
import sys
import types
import unittest
from pathlib import Path


class _Platform:
    def __init__(self, name):
        self.name = name


class _PlatformConfig:
    def __init__(self, *, extra=None, token="token"):
        self.extra = extra or {"server_url": "http://alga.local"}
        self.token = token


class _BasePlatformAdapter:
    def __init__(self, config, platform):
        self.config = config
        self.platform = platform
        self.is_connected = True
        self.handled_messages = []

    def build_source(self, **kwargs):
        return kwargs

    async def handle_message(self, event):
        self.handled_messages.append(event)

    def truncate_message(self, content, max_length):
        return [content[i : i + max_length] for i in range(0, len(content), max_length)]

    def _mark_connected(self):
        self.is_connected = True

    def _mark_disconnected(self):
        self.is_connected = False

    def _set_fatal_error(self, *args, **kwargs):
        pass

    async def _notify_fatal_error(self):
        pass


class _MessageEvent:
    def __init__(self, *, text, message_type, source, raw_message, message_id,
                 reply_to_message_id=None, reply_to_text=None):
        self.text = text
        self.message_type = message_type
        self.source = source
        self.raw_message = raw_message
        self.message_id = message_id
        self.reply_to_message_id = reply_to_message_id
        self.reply_to_text = reply_to_text


class _MessageType:
    TEXT = "text"


class _SendResult:
    def __init__(self, *, success, message_id=None, error=None, retryable=False):
        self.success = success
        self.message_id = message_id
        self.error = error
        self.retryable = retryable


class _MessageDeduplicator:
    def __init__(self, *args, **kwargs):
        self.seen = set()

    def is_duplicate(self, message_id):
        if message_id in self.seen:
            return True
        self.seen.add(message_id)
        return False


class _FakeResponse:
    status_code = 200

    def json(self):
        return {"message_id": "msg-1"}


class _FakeClient:
    def __init__(self):
        self.posts = []

    async def post(self, path, json):
        self.posts.append((path, json))
        return _FakeResponse()


def _load_register_module():
    gateway = types.ModuleType("gateway")
    gateway_config = types.ModuleType("gateway.config")
    gateway_config.Platform = _Platform
    gateway_config.PlatformConfig = _PlatformConfig

    gateway_platforms = types.ModuleType("gateway.platforms")
    gateway_platforms_base = types.ModuleType("gateway.platforms.base")
    gateway_platforms_base.BasePlatformAdapter = _BasePlatformAdapter
    gateway_platforms_base.MessageEvent = _MessageEvent
    gateway_platforms_base.MessageType = _MessageType
    gateway_platforms_base.SendResult = _SendResult

    gateway_platforms_helpers = types.ModuleType("gateway.platforms.helpers")
    gateway_platforms_helpers.MessageDeduplicator = _MessageDeduplicator

    sys.modules.update(
        {
            "gateway": gateway,
            "gateway.config": gateway_config,
            "gateway.platforms": gateway_platforms,
            "gateway.platforms.base": gateway_platforms_base,
            "gateway.platforms.helpers": gateway_platforms_helpers,
        }
    )

    path = Path(__file__).resolve().parents[1] / "plugin" / "register.py"
    spec = importlib.util.spec_from_file_location("alga_register_for_test", path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class ChatIDHelperTests(unittest.IsolatedAsyncioTestCase):
    async def test_complete_investigation_tool_removed(self):
        module = _load_register_module()

        names = {tool["name"] for tool in module._ALGA_TOOLS}

        self.assertNotIn("alga_complete_investigation", names)
        self.assertIn("alga_resolve_alert", names)
        self.assertIn("alga_resolve_incident", names)

    async def test_incoming_owner_chat_id_is_delivered_without_error(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_incoming(
            {
                "chat_id": "alert_42",
                "text": "status?",
                "message_id": "m-1",
                "sender_id": "u-1",
                "sender_name": "Operator",
            }
        )

        self.assertEqual(len(adapter.handled_messages), 1)
        self.assertEqual(adapter.handled_messages[0].source["chat_id"], "alert_42")

    async def test_alert_thread_scopes_agent_to_alert_tools(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_sse_event(
            "message",
            json.dumps({
                "text": "promote to incident",
                "chat_id": "alert_55",
                "message_id": "m-alert-scope-1",
                "sender_id": "u-1",
                "sender_name": "Operator",
            }),
        )

        self.assertEqual(len(adapter.handled_messages), 1)
        text = adapter.handled_messages[0].text
        self.assertIn("ALGA ALERT INVESTIGATION CONTEXT", text)
        self.assertIn("alga_promote_to_incident", text)
        self.assertIn("alga_resolve_alert", text)
        self.assertIn("Do NOT call incident tools", text)
        self.assertNotIn("alga_resolve_incident`", text)

    def test_alert_context_prefix_defers_promotion_to_runbook(self):
        # The alert-investigation context prefix must make the runbook
        # authoritative for promotion: a mandatory-promotion runbook directs the
        # agent to promote per the runbook (before/alongside mitigation), not to
        # impose an extra material-impact gate that overrides it.
        module = _load_register_module()
        prefix = module._alert_investigation_context_prefix("alert_36")

        self.assertIn("promote when EITHER", prefix)
        self.assertIn("mandatory/immediate promotion", prefix)
        self.assertIn("before or alongside mitigation", prefix)
        self.assertIn("Verify the alert before promoting", prefix)
        self.assertIn("do not blindly promote", prefix)
        self.assertIn("user-facing impact", prefix)
        self.assertIn("A runbook is not required", prefix)
        for forbidden in (
            "do NOT auto-promote",
            "does NOT override the material-impact promotion gate",
            "you must call `alga_promote_to_incident` immediately. Do not perform deep validation",
        ):
            self.assertNotIn(forbidden, prefix, f"prefix must not override the runbook: {forbidden}")

    def test_alert_tool_scope_excludes_incident_tools(self):
        module = _load_register_module()

        allowed = module._allowed_alga_tools_for_chat("alert_55")

        self.assertIn("alga_promote_to_incident", allowed)
        self.assertIn("alga_set_outcome", allowed)
        self.assertNotIn("alga_get_incident_context", allowed)
        self.assertNotIn("alga_resolve_incident", allowed)

    async def test_coordination_task_dispatched_activates_agent(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        raw = json.dumps({
            "type": "coordination_task_dispatched",
            "message_id": "m-comms-1",
            "chat_id": "incident_coord_24",
            "text": "Commander dispatched a communicate-kind task for Incident 24.",
            "sender_id": "cmd-1",
            "sender_name": "commander-agent",
            "incident_id": "24",
            "incident_role": "communications_lead",
            "status_level": "investigating",
        })
        await adapter._handle_sse_event("coordination_task_dispatched", raw)

        # The dispatched coordination task must reach the agent as an actionable
        # message so the assigned role (e.g. communicator) acts on it.
        self.assertEqual(len(adapter.handled_messages), 1)
        self.assertEqual(adapter.handled_messages[0].source["chat_id"], "incident_coord_24")

    async def test_incident_comms_task_event_no_longer_handled(self):
        # The legacy incident_comms_task SSE event was removed in favor of
        # coordination_task_dispatched. It must NOT be forwarded as incoming.
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        raw = json.dumps({
            "type": "comms_task",
            "message_id": "m-comms-legacy",
            "chat_id": "incident_coord_24",
            "text": "legacy comms task",
            "sender_id": "cmd-1",
            "sender_name": "commander-agent",
            "incident_role": "communications_lead",
        })
        await adapter._handle_sse_event("incident_comms_task", raw)
        self.assertEqual(len(adapter.handled_messages), 0)

    async def test_unhandled_sse_event_is_ignored(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_sse_event(
            "some_other_event",
            json.dumps({"chat_id": "incident_coord_1", "text": "x", "message_id": "m-ign-1"}),
        )
        self.assertEqual(len(adapter.handled_messages), 0)

    async def test_send_posts_owner_chat_id_without_transport_prefix(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        fake_client = _FakeClient()
        adapter._client = fake_client

        result = await adapter.send("alga:alert_42", "investigating")

        self.assertTrue(result.success)
        self.assertEqual(fake_client.posts[0][1]["chat_id"], "alert_42")

    async def test_post_handoff_tool_payload(self):
        module = _load_register_module()
        fake_calls = []

        async def fake_inv_tool(chat_id, command):
            fake_calls.append((chat_id, command))
            return '{"success": true}'

        module._inv_tool = fake_inv_tool

        result = await module._alga_post_handoff(
            {
                "chat_id": "incident_coord_inc-1",
                "message": "Need command decision on rollback.",
                "audience": "command",
                "urgency": "decision_needed",
                "status_level": "identified",
            }
        )

        self.assertEqual(result, '{"success": true}')
        self.assertEqual(fake_calls[0][0], "incident_coord_inc-1")
        self.assertEqual(
            fake_calls[0][1],
            {
                "op": "post_handoff",
                "message": "Need command decision on rollback.",
                "audience": "command",
                "urgency": "decision_needed",
            },
        )

    async def test_post_handoff_silently_drops_status_level(self):
        # Some LLMs trained on earlier versions of this tool still emit
        # status_level even though the schema no longer declares it. The handler
        # must drop the parameter silently so the LLM sees a successful call
        # instead of a 422 from the backend, and so the backend never sees
        # status_level on a post_handoff payload.
        module = _load_register_module()
        fake_calls = []

        async def fake_inv_tool(chat_id, command):
            fake_calls.append((chat_id, command))
            return '{"success": true}'

        module._inv_tool = fake_inv_tool

        for level in ("investigating", "identified", "mitigated", "monitoring", "resolved"):
            result = await module._alga_post_handoff(
                {
                    "chat_id": "incident_coord_inc-1",
                    "message": f"honing status={level}",
                    "audience": "commander",
                    "urgency": "info",
                    "status_level": level,
                }
            )
            self.assertEqual(result, '{"success": true}', f"unexpected error for status_level={level}")
            self.assertNotIn("status_level", fake_calls[-1][1], f"status_level={level} leaked through to _inv_tool payload")

    async def test_post_handoff_rejects_alert_chat(self):
        module = _load_register_module()

        result = await module._alga_post_handoff(
            {
                "chat_id": "alert_55",
                "message": "Need comms.",
                "audience": "command",
            }
        )

        got = json.loads(result)
        self.assertIn("error", got)
        self.assertIn("incident_", got["error"])

    async def test_incident_tool_descriptions_include_role_guidance(self):
        module = _load_register_module()
        tools = {tool["name"]: tool["schema"]["description"] for tool in module._ALGA_TOOLS}

        self.assertIn("incident commander", tools["alga_resolve_incident"].lower())
        self.assertIn("incident commander", tools["alga_mitigate_incident"].lower())
        self.assertIn("responder", tools["alga_set_incident_severity"].lower())
        self.assertIn("commander-only", tools["alga_dispatch_task"].lower())
        self.assertIn("commander-only", tools["alga_synthesize_findings"].lower())
        self.assertIn("assigned incident roles", tools["alga_get_incident_context"].lower())
        self.assertIn("do not mention internal alert", tools["alga_publish_status_update"].lower())
        coordination = tools["alga_post_handoff"].lower()
        self.assertIn("commander-facing", coordination)
        self.assertIn("audience", coordination)
        self.assertIn("handoff", coordination)
        self.assertNotIn("assigned investigator", coordination)

    def test_incident_role_context_prefix(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("incident_commander", "22", "active", chat_id="incident_coord_22")
        self.assertIn("Incident Commander", prefix)
        self.assertIn("incident_coord_22", prefix)
        self.assertIn("active", prefix)
        self.assertIn("alga_resolve_incident", prefix)
        # Commander owns alert closure as part of incident closure.
        self.assertIn("alga_resolve_alert", prefix)
        self.assertIn("alga_reopen_alert", prefix)
        # The commander must NOT be told it is forbidden from alert tools (old rule).
        self.assertNotIn("forbidden from accessing alert tools", prefix)

    def test_incident_role_context_prefix_mention_format_guidance(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("communications_lead", "28", "active", chat_id="incident_coord_28")
        # Must steer the agent away from leaking IDs and from invalid @ic/@comms mentions.
        self.assertIn("NUMBER only", prefix)
        self.assertIn("never mention or surface investigation IDs", prefix)
        self.assertIn("@ic", prefix)
        self.assertIn("[@Name](agent:UUID)", prefix)
        self.assertIn("123e4567-e89b-12d3-a456-426614174000", prefix)
        self.assertIn("never wrap the UUID in angle brackets", prefix)

    def test_incident_role_context_prefix_responder_coordination_guidance(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("responder", "22", "active", chat_id="incident_coord_22")
        self.assertIn("Responder", prefix)
        self.assertIn("COORDINATION", prefix)
        self.assertIn("do NOT post technical investigation logs", prefix)

    def test_incident_role_context_prefix_responder_forbids_alert_resolution(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("responder", "22", "active", chat_id="incident_coord_22")
        # In incident scope, alert closure is owned by the commander. The responder
        # must be told explicitly not to call alga_resolve_alert / alga_reopen_alert.
        self.assertIn("FORBIDDEN from calling alga_resolve_alert", prefix)
        self.assertIn("alert closure is part of incident closure and is owned by the incident commander", prefix)
        self.assertIn("FORBIDDEN from calling alga_who_is_on_call", prefix)
        self.assertIn("the handoff in the investigation thread (using alga_post_handoff with audience='commander') is always directly to the incident commander", prefix)
        # The role-filtered tool list must NOT include alga_resolve_alert for the responder.
        self.assertIn("Allowed Alga tools in this incident thread", prefix)
        self.assertIn("alga_publish_status_update", prefix)
        self.assertNotIn("alga_resolve_alert`", prefix)
        self.assertNotIn("alga_reopen_alert`", prefix)
        self.assertNotIn("alga_who_is_on_call`", prefix)

    def test_incident_role_context_prefix_commander_advertises_alert_tools(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("incident_commander", "22", "active", chat_id="incident_coord_22")
        # Commander is in the allowed-tools list for the incident thread.
        self.assertIn("Allowed Alga tools in this incident thread", prefix)
        self.assertIn("`alga_resolve_alert`", prefix)
        self.assertIn("`alga_reopen_alert`", prefix)

    def test_allowed_alga_tools_filters_resolve_alert_for_responder(self):
        module = _load_register_module()
        # In alert-investigation scope, resolve_alert is exposed as normal.
        self.assertIn("alga_resolve_alert", module._allowed_alga_tools_for_chat("alert_42"))
        # In incident scope for the responder, resolve_alert, reopen_alert, and who_is_on_call
        # must be filtered out.
        responder_tools = module._allowed_alga_tools_for_chat("incident_coord_22", incident_role="responder")
        self.assertNotIn("alga_resolve_alert", responder_tools)
        self.assertNotIn("alga_reopen_alert", responder_tools)
        self.assertNotIn("alga_who_is_on_call", responder_tools)
        # Commander keeps both tools.
        commander_tools = module._allowed_alga_tools_for_chat("incident_coord_22", incident_role="incident_commander")
        self.assertIn("alga_resolve_alert", commander_tools)
        self.assertIn("alga_reopen_alert", commander_tools)
        self.assertIn("alga_who_is_on_call", commander_tools)

    def test_incident_role_context_prefix_responder_forbids_coordination_during_investigation(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("responder", "22", "active", chat_id="incident_coord_22")
        # The responder must not call alga_post_handoff while still investigating,
        # identifying, mitigating, or verifying — only the single final commander handoff is allowed.
        self.assertIn("NON-NEGOTIABLE", prefix)
        self.assertIn("ACTIVATES other agents", prefix)
        self.assertIn("FORBIDDEN from calling alga_post_handoff", prefix)
        self.assertIn(
            "ALL status communication while you are still working MUST go through alga_publish_status_update",
            prefix,
        )
        self.assertIn("at most ONCE", prefix)
        # Status-level discipline: responder cannot publish resolved or investigating.
        self.assertIn(
            "FORBIDDEN from publishing status_level='resolved' (commander-only) or status_level='investigating' (system-only)",
            prefix,
        )

    def test_alga_post_handoff_description_warns_about_activation(self):
        module = _load_register_module()
        tools = {tool["name"]: tool["schema"]["description"] for tool in module._ALGA_TOOLS}
        desc = tools["alga_post_handoff"]
        self.assertIn("ACTIVATES other agents", desc)
        self.assertTrue("FORBIDDEN" in desc or "forbidden" in desc or "Do NOT" in desc, desc)
        self.assertIn("alga_publish_status_update", desc)

    def test_alga_publish_status_update_description_clarifies_no_activation(self):
        module = _load_register_module()
        tools = {tool["name"]: tool["schema"]["description"] for tool in module._ALGA_TOOLS}
        desc = tools["alga_publish_status_update"]
        self.assertIn("does NOT activate or notify other agents", desc)
        self.assertIn("Responders MUST NOT publish status_level='resolved'", desc)

    def test_incident_role_context_prefix_empty_role(self):
        module = _load_register_module()
        prefix = module._incident_role_context_prefix("", "22", "active", chat_id="incident_coord_22")
        self.assertEqual(prefix, "")


class InvToolMetadataTests(unittest.IsolatedAsyncioTestCase):
    async def test_inv_tool_preserves_incident_metadata(self):
        module = _load_register_module()

        async def fake_agent_post(path, body):
            return {
                "ok": True,
                "op": "promote_to_incident",
                "incident_id": "b94864c5-0491-42d4-9fab-901c563d3afd",
                "incident_number": 21,
            }

        module._agent_post = fake_agent_post
        raw = await module._inv_tool("alert_35", {"op": "promote_to_incident"})
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertEqual(got["incident_id"], "b94864c5-0491-42d4-9fab-901c563d3afd")
        self.assertEqual(got["incident_number"], 21)
        # The backend no longer surfaces the incident investigation id (it is not
        # user-facing or linkable); the plugin must not relay it either.
        self.assertNotIn("incident_investigation_id", got)

    async def test_inv_tool_metadata_absent_when_not_promotion(self):
        module = _load_register_module()

        async def fake_agent_post(path, body):
            return {"ok": True, "op": "set_outcome"}

        module._agent_post = fake_agent_post
        raw = await module._inv_tool("alert_35", {"op": "set_outcome"})
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertNotIn("incident_id", got)
        self.assertNotIn("incident_number", got)


class StopSuppressionTests(unittest.IsolatedAsyncioTestCase):
    async def test_stop_marks_chat_stopped_and_routes_to_handler(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        handled = []

        async def fake_handle_incoming(data):
            handled.append(data)

        adapter._handle_incoming = fake_handle_incoming

        await adapter._handle_sse_event("message", json.dumps({
            "trigger": "observe",
            "text": "/stop",
            "chat_id": "alert_35",
            "message_id": "msg-stop-1",
        }))

        self.assertEqual(len(handled), 1)
        self.assertTrue(adapter._is_chat_stopped("alert_35"))

    async def test_send_suppressed_for_stopped_chat(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        fake_client = _FakeClient()
        adapter._client = fake_client
        adapter._mark_chat_stopped("alert_35")

        result = await adapter.send("alert_35", "late output from stopped run")

        self.assertTrue(result.success)
        self.assertIsNone(result.message_id)
        self.assertEqual(len(fake_client.posts), 0)

    async def test_send_allowed_for_non_stopped_chat(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        fake_client = _FakeClient()
        adapter._client = fake_client

        result = await adapter.send("alert_35", "normal output")

        self.assertTrue(result.success)
        self.assertEqual(len(fake_client.posts), 1)

    async def test_dispatch_clears_stopped_state(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        handled = []

        async def fake_handle_incoming(data):
            handled.append(data)

        adapter._handle_incoming = fake_handle_incoming
        adapter._mark_chat_stopped("alert_35")

        await adapter._handle_sse_event("message", json.dumps({
            "trigger": "dispatch",
            "text": "New investigation: Alert #36",
            "chat_id": "alert_35",
            "message_id": "msg-dispatch-1",
        }))

        self.assertFalse(adapter._is_chat_stopped("alert_35"))
        self.assertEqual(len(handled), 1)

    async def test_mention_clears_stopped_state(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        handled = []

        async def fake_handle_incoming(data):
            handled.append(data)

        adapter._handle_incoming = fake_handle_incoming
        adapter._mark_chat_stopped("alert_35")

        await adapter._handle_sse_event("message", json.dumps({
            "trigger": "mention",
            "text": "What's the status?",
            "chat_id": "alert_35",
            "message_id": "msg-mention-1",
        }))

        self.assertFalse(adapter._is_chat_stopped("alert_35"))

    async def test_non_stop_observe_does_not_mark_stopped(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_sse_event("message", json.dumps({
            "trigger": "observe",
            "text": "just watching",
            "chat_id": "alert_35",
            "message_id": "msg-observe-1",
        }))

        self.assertFalse(adapter._is_chat_stopped("alert_35"))


class CreateKnowledgeRequirementsTests(unittest.IsolatedAsyncioTestCase):
    async def test_create_knowledge_requires_source_investigation_id(self):
        module = _load_register_module()
        raw = await module._alga_create_knowledge({
            "title": "Test",
            "body_markdown": "Body",
            "confidence": 0.9,
        })
        got = json.loads(raw)
        self.assertIn("error", got)
        self.assertIn("source_investigation_id", got["error"])

    async def test_create_knowledge_requires_confidence(self):
        module = _load_register_module()
        raw = await module._alga_create_knowledge({
            "title": "Test",
            "body_markdown": "Body",
            "source_investigation_id": "ainv-1",
        })
        got = json.loads(raw)
        self.assertIn("error", got)
        self.assertIn("confidence", got["error"])

    async def test_create_knowledge_sends_required_fields(self):
        module = _load_register_module()
        captured = {}

        async def fake_agent_post(path, body):
            captured["body"] = body
            return {"id": "note-1"}

        module._agent_post = fake_agent_post
        raw = await module._alga_create_knowledge({
            "kind": "runbook",
            "title": "PostgreSQLDown Recovery",
            "body_markdown": "Mitigate first.",
            "source_investigation_id": "5dc4b2a6-4af1-4b0f-8ff3-bdca3042c104",
            "confidence": 0.95,
        })
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertEqual(captured["body"]["source_investigation_id"], "5dc4b2a6-4af1-4b0f-8ff3-bdca3042c104")
        self.assertEqual(captured["body"]["confidence"], 0.95)

    async     def test_create_knowledge_schema_requires_source_and_confidence(self):
        module = _load_register_module()
        schema = {t["name"]: t["schema"] for t in module._ALGA_TOOLS}["alga_create_knowledge"]
        params = schema["parameters"]
        required = params["required"]
        self.assertIn("source_investigation_id", required)
        self.assertIn("confidence", required)
        self.assertIn("source_investigation_id", params["properties"])


class GetKnowledgeTests(unittest.IsolatedAsyncioTestCase):
    async def test_get_knowledge_requires_id(self):
        module = _load_register_module()
        raw = await module._alga_get_knowledge({})
        got = json.loads(raw)
        self.assertIn("error", got)
        self.assertIn("id", got["error"])

    async def test_get_knowledge_fetches_full_note(self):
        module = _load_register_module()
        captured = {}

        async def fake_agent_get(path, params=None):
            captured["path"] = path
            return {
                "id": "00000000-0000-0000-0000-0000000000aa",
                "kind": "runbook",
                "title": "PostgreSQLDown on Patroni-managed nodes",
                "body_markdown": "## Full body\nMuch longer than 200 chars." * 5,
            }

        module._agent_get = fake_agent_get
        raw = await module._alga_get_knowledge({"id": "00000000-0000-0000-0000-0000000000aa"})
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertIn("/agent/knowledge/", captured["path"])
        self.assertIn("00000000-0000-0000-0000-0000000000aa", captured["path"])
        self.assertIn("Full body", got["note"]["body_markdown"])

    async def test_get_knowledge_registered_in_tools_and_handlers(self):
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        self.assertIn("alga_get_knowledge", names)
        self.assertIn("alga_get_knowledge", module._TOOL_HANDLERS)

    async def test_get_knowledge_schema_requires_id(self):
        module = _load_register_module()
        schema = {t["name"]: t["schema"] for t in module._ALGA_TOOLS}["alga_get_knowledge"]
        self.assertIn("id", schema["parameters"]["required"])

    async def test_search_knowledge_returns_note_id(self):
        module = _load_register_module()

        async def fake_agent_get(path, params=None):
            return {
                "items": [
                    {
                        "id": "00000000-0000-0000-0000-0000000000aa",
                        "kind": "runbook",
                        "title": "PostgreSQLDown Recovery",
                        "body_markdown": "x" * 500,
                    }
                ],
                "total": 1,
            }

        module._agent_get = fake_agent_get
        raw = await module._alga_search_knowledge({"query": "postgres"})
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertIn("00000000-0000-0000-0000-0000000000aa", got["notes"])
        self.assertIn("truncated", got["notes"].lower())
        self.assertIn("alga_get_knowledge", got.get("hint", ""))


class ResolutionDocsTests(unittest.IsolatedAsyncioTestCase):
    async def test_set_resolution_docs_requires_a_field(self):
        module = _load_register_module()
        raw = await module._alga_set_incident_resolution_docs({"incident_id": "42"})
        got = json.loads(raw)
        self.assertIn("error", got)
        self.assertIn("at least one of", got["error"])

    async def test_set_resolution_docs_sends_inv_tool_command(self):
        module = _load_register_module()
        captured = {}

        async def fake_inv_tool(chat_id, command):
            captured["chat_id"] = chat_id
            captured["command"] = command
            return '{"success": true}'

        module._inv_tool = fake_inv_tool
        raw = await module._alga_set_incident_resolution_docs({
            "incident_id": "42",
            "summary": "Resolved.",
            "impact_assessment": "No impact.",
            "actions_taken": "Verified.",
            "root_cause": "Bad deploy bypassed canary.",
            "resolution": "Rolled back the deploy.",
        })
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertEqual(captured["chat_id"], "incident_coord_42")
        self.assertEqual(captured["command"]["op"], "set_incident_resolution_docs")
        self.assertEqual(captured["command"]["incident_id"], "42")
        self.assertEqual(captured["command"]["summary"], "Resolved.")
        self.assertEqual(captured["command"]["impact_assessment"], "No impact.")
        self.assertEqual(captured["command"]["actions_taken"], "Verified.")
        self.assertEqual(captured["command"]["root_cause"], "Bad deploy bypassed canary.")
        self.assertEqual(captured["command"]["resolution"], "Rolled back the deploy.")

    async def test_resolve_incident_passes_inline_resolution_fields(self):
        module = _load_register_module()
        captured = {}

        async def fake_inv_tool(chat_id, command):
            captured["command"] = command
            return '{"success": true}'

        module._inv_tool = fake_inv_tool
        raw = await module._alga_resolve_incident({
            "incident_id": "24",
            "reason": "closed",
            "summary": "S.",
            "impact_assessment": "I.",
            "actions_taken": "A.",
            "root_cause": "RC.",
            "resolution": "RES.",
        })
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertEqual(captured["command"]["op"], "resolve_incident")
        self.assertEqual(captured["command"]["reason"], "closed")
        self.assertEqual(captured["command"]["summary"], "S.")
        self.assertEqual(captured["command"]["impact_assessment"], "I.")
        self.assertEqual(captured["command"]["actions_taken"], "A.")
        self.assertEqual(captured["command"]["root_cause"], "RC.")
        self.assertEqual(captured["command"]["resolution"], "RES.")

    async def test_resolve_incident_omits_blank_resolution_fields(self):
        module = _load_register_module()
        captured = {}

        async def fake_inv_tool(chat_id, command):
            captured["command"] = command
            return '{"success": true}'

        module._inv_tool = fake_inv_tool
        await module._alga_resolve_incident({"incident_id": "24", "reason": "  "})
        self.assertNotIn("summary", captured["command"])
        self.assertNotIn("impact_assessment", captured["command"])
        self.assertNotIn("actions_taken", captured["command"])
        self.assertNotIn("root_cause", captured["command"])
        self.assertNotIn("resolution", captured["command"])
        self.assertNotIn("reason", captured["command"])

    async def test_resolution_docs_tool_registered(self):
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        self.assertIn("alga_set_incident_resolution_docs", names)
        self.assertIn("alga_set_incident_resolution_docs", module._TOOL_HANDLERS)

    async def test_request_status_update_tool_removed(self):
        # L6 removed alga_request_status_update in favor of alga_dispatch_task.
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        self.assertNotIn("alga_request_status_update", names)
        self.assertNotIn("alga_request_status_update", module._TOOL_HANDLERS)
        self.assertFalse(hasattr(module, "_alga_request_status_update"))

    async def test_investigate_gated_tools_advertise_capability(self):
        module = _load_register_module()
        descs = {t["name"]: t["schema"]["description"].lower() for t in module._ALGA_TOOLS}
        for name in ("alga_get_incident_context", "alga_list_alerts", "alga_add_incident_timeline"):
            self.assertIn("investigate", descs[name], f"{name} should note investigate requirement")
        self.assertIn("must not use this tool", descs["alga_set_outcome"])

    async def test_report_to_communicator_tool_removed(self):
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        self.assertNotIn("alga_report_to_communicator", names)
        self.assertNotIn("alga_report_to_communicator", module._TOOL_HANDLERS)
        self.assertFalse(hasattr(module, "_alga_report_to_communicator"))

    async def test_publish_status_update_tool_registered_and_routes(self):
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        self.assertIn("alga_publish_status_update", names)
        self.assertIn("alga_publish_status_update", module._TOOL_HANDLERS)

        captured = {}

        async def fake_inv_tool(chat_id, command):
            captured["chat_id"] = chat_id
            captured["command"] = command
            return '{"success": true}'

        module._inv_tool = fake_inv_tool
        raw = await module._alga_publish_status_update({
            "incident_id": "24",
            "message": "We have identified the cause and are rolling out a fix.",
            "status_level": "identified",
            "source_coordination_message_id": "msg-7",
        })
        got = json.loads(raw)
        self.assertTrue(got["success"])
        self.assertEqual(captured["chat_id"], "incident_coord_24")
        self.assertEqual(captured["command"]["op"], "publish_status_update")
        self.assertEqual(captured["command"]["status_level"], "identified")
        self.assertEqual(captured["command"]["source_coordination_message_id"], "msg-7")

    async def test_publish_status_update_requires_valid_status_level(self):
        module = _load_register_module()
        raw = await module._alga_publish_status_update({
            "incident_id": "24",
            "message": "x",
            "status_level": "bogus",
        })
        got = json.loads(raw)
        self.assertIn("error", got)

    async def test_resolve_incident_description_notes_status_update_requirement(self):
        module = _load_register_module()
        desc = {t["name"]: t["schema"]["description"].lower() for t in module._ALGA_TOOLS}
        self.assertIn("status update", desc["alga_resolve_incident"])
        self.assertIn("alga_publish_status_update", desc["alga_resolve_incident"])


class _FakeSession:
    def __init__(self):
        self.session_id = "s-1"
        self.entries = []

    def get_or_create_session(self, source):
        return self

    def append_to_transcript(self, session_id, entry):
        self.entries.append(entry)


class ReplyToMessageTests(unittest.IsolatedAsyncioTestCase):
    async def test_inbound_reply_to_populates_message_event_fields(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_incoming(
            {
                "chat_id": "alert_42",
                "text": "That was the deploy.",
                "message_id": "m-2",
                "sender_id": "u-1",
                "sender_name": "Operator",
                "reply_to_message_id": "m-1",
                "reply_to_text": "Was it the config change?",
            }
        )

        self.assertEqual(len(adapter.handled_messages), 1)
        event = adapter.handled_messages[0]
        self.assertEqual(event.reply_to_message_id, "m-1")
        self.assertEqual(event.reply_to_text, "Was it the config change?")

    async def test_inbound_without_reply_to_leaves_fields_none(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_incoming(
            {
                "chat_id": "alert_42",
                "text": "plain message",
                "message_id": "m-1",
                "sender_id": "u-1",
                "sender_name": "Operator",
            }
        )

        self.assertEqual(len(adapter.handled_messages), 1)
        event = adapter.handled_messages[0]
        self.assertIsNone(event.reply_to_message_id)
        self.assertIsNone(event.reply_to_text)

    async def test_inbound_reply_to_with_empty_id_is_ignored(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())

        await adapter._handle_incoming(
            {
                "chat_id": "alert_42",
                "text": "stray reply body",
                "message_id": "m-1",
                "sender_id": "u-1",
                "sender_name": "Operator",
                "reply_to_text": "some reply text without an id",
            }
        )

        self.assertEqual(len(adapter.handled_messages), 1)
        event = adapter.handled_messages[0]
        self.assertIsNone(event.reply_to_message_id)
        self.assertIsNone(event.reply_to_text)

    async def test_send_forwards_reply_to_message_id(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        fake_client = _FakeClient()
        adapter._client = fake_client

        result = await adapter.send("alert_42", "reply body", reply_to="msg-99")

        self.assertTrue(result.success)
        self.assertEqual(fake_client.posts[0][1]["reply_to_message_id"], "msg-99")

    async def test_send_without_reply_to_omits_field(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        fake_client = _FakeClient()
        adapter._client = fake_client

        result = await adapter.send("alert_42", "body")

        self.assertTrue(result.success)
        self.assertNotIn("reply_to_message_id", fake_client.posts[0][1])

    async def test_send_multichunk_only_first_chunk_carries_reply_to(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        fake_client = _FakeClient()
        adapter._client = fake_client

        big_body = "x" * (module.MAX_MESSAGE_LENGTH + 10)
        result = await adapter.send("alert_42", big_body, reply_to="msg-99")

        self.assertTrue(result.success)
        self.assertGreater(len(fake_client.posts), 1)
        self.assertEqual(fake_client.posts[0][1]["reply_to_message_id"], "msg-99")
        for post in fake_client.posts[1:]:
            self.assertNotIn("reply_to_message_id", post[1])

    async def test_observe_records_reply_to_in_transcript(self):
        module = _load_register_module()
        adapter = module.AlgaAdapter(_PlatformConfig())
        session = _FakeSession()
        adapter._session_store = session

        await adapter._observe_message(
            {
                "chat_id": "alert_42",
                "text": "the actual reply",
                "message_id": "m-2",
                "sender_id": "u-1",
                "sender_name": "Operator",
                "reply_to_message_id": "m-1",
                "reply_to_text": "the original question",
            }
        )

        self.assertEqual(len(session.entries), 1)
        entry = session.entries[0]
        self.assertTrue(entry["content"].startswith("[Replying to:"))
        self.assertIn("the original question", entry["content"])
        self.assertIn("the actual reply", entry["content"])


if __name__ == "__main__":
    unittest.main()
