/**
 * Telemetry Logger — Webhook External Plugin
 *
 * Receives telemetry events from the platform via HTTP POST (JSON-RPC)
 * and logs them to console. Sends periodic heartbeats.
 *
 * Usage:
 *   java -cp target/classes TelemetryLogger --platform http://localhost:8080 --token <admin-jwt> --port 9000
 *
 * Dependencies (Maven):
 *   - com.google.code.gson:gson:2.10.1
 *   - com.sun.net.httpserver (stdlib)
 *
 * The plugin:
 * 1. Registers with the platform using admin JWT
 * 2. Starts HTTP server on specified port
 * 3. Receives plugin.on_event, plugin.apply_config, plugin.detach via JSON-RPC
 * 4. Sends heartbeat every 30 seconds
 */

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.InetAddress;
import java.nio.charset.StandardCharsets;
import java.time.LocalTime;
import java.time.format.DateTimeFormatter;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicLong;

public class TelemetryLogger {

    // ─── Configuration ───────────────────────────────────────────────────────

    private static final String PLUGIN_ID = "telemetry-logger";
    private static final String PLUGIN_NAME = "Telemetry Logger";
    private static final String PLUGIN_DESC = "Logs device telemetry to console via webhook";
    private static final DateTimeFormatter TIME_FMT = DateTimeFormatter.ofPattern("HH:mm:ss");

    // ─── Globals ─────────────────────────────────────────────────────────────

    private static String pluginToken = "";
    private static final AtomicLong eventsCount = new AtomicLong(0);
    private static final long startTime = System.currentTimeMillis();
    private static final Gson gson = new Gson();

    // ─── Registration ────────────────────────────────────────────────────────

    static String register(String platformUrl, String jwtToken, String endpoint) throws Exception {
        JsonObject payload = new JsonObject();
        payload.addProperty("id", PLUGIN_ID);
        payload.addProperty("name", PLUGIN_NAME);
        payload.addProperty("description", PLUGIN_DESC);
        payload.addProperty("endpoint", endpoint);
        payload.addProperty("auto", false);
        payload.addProperty("interval_ms", 0);
        payload.add("default_config", new JsonObject());
        payload.add("form", gson.toJsonTree(new Object[]{}));

        URI uri = URI.create(platformUrl + "/api/v1/plugins/external");
        HttpURLConnection conn = (HttpURLConnection) uri.toURL().openConnection();
        conn.setRequestMethod("POST");
        conn.setRequestProperty("Content-Type", "application/json");
        conn.setRequestProperty("Authorization", "Bearer " + jwtToken);
        conn.setDoOutput(true);
        conn.setConnectTimeout(10000);

        try (OutputStream os = conn.getOutputStream()) {
            os.write(payload.toString().getBytes(StandardCharsets.UTF_8));
        }

        int status = conn.getResponseCode();
        if (status != 201) {
            throw new RuntimeException("Registration failed: HTTP " + status);
        }

        String body = new String(conn.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
        JsonObject resp = JsonParser.parseString(body).getAsJsonObject();
        String token = resp.get("token").getAsString();
        System.out.printf("[%s] Registered: id=%s token=%s...%n",
                LocalTime.now().format(TIME_FMT), resp.get("id").getAsString(), token.substring(0, 16));
        return token;
    }

    // ─── Heartbeat ───────────────────────────────────────────────────────────

    static void heartbeatLoop(String platformUrl) {
        while (true) {
            try {
                Thread.sleep(30000);

                JsonObject payload = new JsonObject();
                payload.addProperty("jsonrpc", "2.0");
                payload.addProperty("id", 1);
                payload.addProperty("method", "platform.heartbeat");

                JsonObject params = new JsonObject();
                params.addProperty("plugin_id", PLUGIN_ID);
                params.addProperty("status", "ok");

                JsonObject stats = new JsonObject();
                stats.addProperty("events_processed", eventsCount.get());
                stats.addProperty("uptime_sec", (System.currentTimeMillis() - startTime) / 1000);
                params.add("stats", stats);

                payload.add("params", params);

                URI uri = URI.create(platformUrl + "/api/plugins/external/rpc");
                HttpURLConnection conn = (HttpURLConnection) uri.toURL().openConnection();
                conn.setRequestMethod("POST");
                conn.setRequestProperty("Content-Type", "application/json");
                conn.setRequestProperty("Authorization", "Bearer " + pluginToken);
                conn.setDoOutput(true);
                conn.setConnectTimeout(5000);

                try (OutputStream os = conn.getOutputStream()) {
                    os.write(payload.toString().getBytes(StandardCharsets.UTF_8));
                }

                int status = conn.getResponseCode();
                if (status == 200) {
                    System.out.printf("[%s] Heartbeat OK (events: %d)%n",
                            LocalTime.now().format(TIME_FMT), eventsCount.get());
                } else {
                    System.out.printf("[%s] Heartbeat failed: %d%n",
                            LocalTime.now().format(TIME_FMT), status);
                }
                conn.disconnect();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                System.out.printf("[%s] Heartbeat failed: %s%n",
                        LocalTime.now().format(TIME_FMT), e.getMessage());
            }
        }
    }

    // ─── HTTP Handler ────────────────────────────────────────────────────────

    static void handleRpc(HttpExchange exchange) throws IOException {
        String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
        JsonObject req = JsonParser.parseString(body).getAsJsonObject();
        String method = req.get("method").getAsString();
        JsonElement id = req.get("id");
        JsonObject params = req.has("params") ? req.getAsJsonObject("params") : new JsonObject();

        String response;

        switch (method) {
            case "plugin.on_event":
                response = handleOnEvent(id, params);
                break;
            case "plugin.apply_config":
                response = handleApplyConfig(id, params);
                break;
            case "plugin.detach":
                response = handleDetach(id, params);
                break;
            default:
                response = makeError(id, -32601, "unknown method: " + method);
                break;
        }

        byte[] bytes = response.getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.sendResponseHeaders(200, bytes.length);
        try (OutputStream os = exchange.getResponseBody()) {
            os.write(bytes);
        }
    }

    static String handleOnEvent(JsonElement id, JsonObject params) {
        eventsCount.incrementAndGet();

        long bindingId = params.has("binding_id") ? params.get("binding_id").getAsLong() : 0;
        long devicePk = params.has("device_pk") ? params.get("device_pk").getAsLong() : 0;
        String kind = params.has("kind") ? params.get("kind").getAsString() : "unknown";
        JsonElement payload = params.has("payload") ? params.get("payload") : null;

        String ts = LocalTime.now().format(TIME_FMT);

        switch (kind) {
            case "telemetry":
                System.out.printf("📊 Device #%d (binding %d):%n", devicePk, bindingId);
                if (payload != null && payload.isJsonObject()) {
                    for (var entry : payload.getAsJsonObject().entrySet()) {
                        System.out.printf("   %s: %s%n", entry.getKey(), entry.getValue());
                    }
                }
                break;
            case "status":
                boolean online = params.has("online") && params.get("online").getAsBoolean();
                System.out.printf("🔌 Device #%d: %s%n", devicePk, online ? "online" : "offline");
                break;
            case "event":
                System.out.printf("⚡ Device #%d event: %s%n", devicePk, payload);
                break;
            default:
                System.out.printf("📨 Device #%d [%s]: %s%n", devicePk, kind, payload);
                break;
        }

        return makeResult(id, "{\"ok\":true}");
    }

    static String handleApplyConfig(JsonElement id, JsonObject params) {
        long bindingId = params.has("binding_id") ? params.get("binding_id").getAsLong() : 0;
        System.out.printf("Binding %d: config applied%n", bindingId);
        return makeResult(id, "{\"ok\":true}");
    }

    static String handleDetach(JsonElement id, JsonObject params) {
        long bindingId = params.has("binding_id") ? params.get("binding_id").getAsLong() : 0;
        System.out.printf("Binding %d: detached%n", bindingId);
        return makeResult(id, "{\"ok\":true}");
    }

    // ─── JSON-RPC Helpers ────────────────────────────────────────────────────

    static String makeResult(JsonElement id, String result) {
        return String.format("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%s}", id, result);
    }

    static String makeError(JsonElement id, int code, String message) {
        return String.format("{\"jsonrpc\":\"2.0\",\"id\":%s,\"error\":{\"code\":%d,\"message\":\"%s\"}}",
                id, code, message.replace("\"", "\\\""));
    }

    // ─── Main ────────────────────────────────────────────────────────────────

    public static void main(String[] args) throws Exception {
        String platformUrl = "http://localhost:8080";
        String token = null;
        int port = 9000;

        for (int i = 0; i < args.length; i++) {
            switch (args[i]) {
                case "--platform": platformUrl = args[++i]; break;
                case "--token": token = args[++i]; break;
                case "--port": port = Integer.parseInt(args[++i]); break;
            }
        }

        if (token == null) {
            System.err.println("--token is required (admin JWT)");
            System.exit(1);
        }

        System.out.println("==================================================");
        System.out.println("Telemetry Logger Plugin (Webhook)");
        System.out.println("==================================================");

        // Determine endpoint
        String hostname = InetAddress.getLocalHost().getHostName();
        String endpoint = String.format("http://%s:%d/rpc", hostname, port);

        // Register
        pluginToken = register(platformUrl, token, endpoint);

        // Start heartbeat thread
        Thread heartbeatThread = new Thread(() -> heartbeatLoop(platformUrl));
        heartbeatThread.setDaemon(true);
        heartbeatThread.start();

        // Start HTTP server
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/rpc", TelemetryLogger::handleRpc);
        server.setExecutor(Executors.newFixedThreadPool(4));

        // Graceful shutdown
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutting down...");
            server.stop(1);
        }));

        server.start();
        System.out.printf("Listening on port %d...%n", port);
        System.out.println("Waiting for events from platform...");
        System.out.println("--------------------------------------------------");
    }
}
