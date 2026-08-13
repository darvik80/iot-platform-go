/**
 * MQTT Telemetry Logger — External Plugin (Node-RED style)
 *
 * Connects to the platform MQTT broker directly, subscribes to rules bus events,
 * and logs all telemetry/status/events to console.
 *
 * Usage:
 *   java -cp target/classes MqttTelemetryLogger --platform http://localhost:8080 --token <admin-jwt>
 *
 * Dependencies (Maven):
 *   - org.eclipse.paho:org.eclipse.paho.client.mqttv3:1.2.5
 *   - com.google.code.gson:gson:2.10.1
 *
 * The plugin:
 * 1. Registers with the platform using admin JWT (gets API token + MQTT credentials)
 * 2. Connects to MQTT broker using the provided credentials
 * 3. Subscribes to platform/v1/events/# (all device events)
 * 4. Logs everything to console
 */

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import org.eclipse.paho.client.mqttv3.*;
import org.eclipse.paho.client.mqttv3.persist.MemoryPersistence;

import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.LocalTime;
import java.time.format.DateTimeFormatter;
import java.util.HashMap;
import java.util.Map;

public class MqttTelemetryLogger {

    // ─── Configuration ───────────────────────────────────────────────────────

    private static final String PLUGIN_ID = "mqtt-telemetry-logger";
    private static final String PLUGIN_NAME = "MQTT Telemetry Logger";
    private static final String PLUGIN_DESC = "Subscribes to rules bus via MQTT and logs all device telemetry";
    private static final DateTimeFormatter TIME_FMT = DateTimeFormatter.ofPattern("HH:mm:ss");

    // ─── Registration ────────────────────────────────────────────────────────

    static class RegistrationResponse {
        boolean ok;
        String id;
        String token;
        String mqtt_user;
        String mqtt_pass;
        String mqtt_host;
        int mqtt_port;
        String rules_bus;
    }

    static RegistrationResponse register(String platformUrl, String jwtToken) throws Exception {
        JsonObject payload = new JsonObject();
        payload.addProperty("id", PLUGIN_ID);
        payload.addProperty("name", PLUGIN_NAME);
        payload.addProperty("description", PLUGIN_DESC);

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
        RegistrationResponse resp = new Gson().fromJson(body, RegistrationResponse.class);
        System.out.printf("[%s] Registered: id=%s mqtt_user=%s%n",
                LocalTime.now().format(TIME_FMT), resp.id, resp.mqtt_user);
        return resp;
    }

    // ─── MQTT Client ─────────────────────────────────────────────────────────

    static class MQTTLoggerPlugin {
        private final RegistrationResponse config;
        private MqttClient client;
        private final Gson gson = new Gson();

        MQTTLoggerPlugin(RegistrationResponse config) {
            this.config = config;
        }

        void connect() throws Exception {
            String clientId = "extplg_" + PLUGIN_ID;
            String broker = String.format("tcp://%s:%d", config.mqtt_host, config.mqtt_port);

            client = new MqttClient(broker, clientId, new MemoryPersistence());

            MqttConnectOptions opts = new MqttConnectOptions();
            opts.setUserName(config.mqtt_user);
            opts.setPassword(config.mqtt_pass.toCharArray());
            opts.setKeepAliveInterval(60);
            opts.setCleanSession(true);
            opts.setAutomaticReconnect(true);
            opts.setConnectionTimeout(10);

            client.setCallback(new MqttCallbackExtended() {
                @Override
                public void connectComplete(boolean reconnect, String serverURI) {
                    System.out.printf("[%s] MQTT connected%s%n",
                            LocalTime.now().format(TIME_FMT), reconnect ? " (reconnect)" : "");

                    String topic = config.rules_bus + "/events/#";
                    try {
                        client.subscribe(topic, 0);
                        System.out.printf("[%s] Subscribed to %s%n",
                                LocalTime.now().format(TIME_FMT), topic);
                    } catch (MqttException e) {
                        System.err.printf("[%s] Subscribe failed: %s%n",
                                LocalTime.now().format(TIME_FMT), e.getMessage());
                    }
                }

                @Override
                public void connectionLost(Throwable cause) {
                    System.out.printf("[%s] MQTT disconnected: %s%n",
                            LocalTime.now().format(TIME_FMT), cause.getMessage());
                }

                @Override
                public void messageArrived(String topic, MqttMessage message) {
                    handleMessage(topic, new String(message.getPayload(), StandardCharsets.UTF_8));
                }

                @Override
                public void deliveryComplete(IMqttDeliveryToken token) {
                }
            });

            System.out.printf("[%s] Connecting to MQTT broker %s as %s%n",
                    LocalTime.now().format(TIME_FMT), broker, config.mqtt_user);
            client.connect(opts);
        }

        void handleMessage(String topic, String payload) {
            // Parse topic: platform/v1/events/{product}/{device}/{kind}
            String[] parts = topic.split("/");
            String kind = parts.length > 3 ? parts[parts.length - 1] : "unknown";
            String device = parts.length > 3 ? parts[parts.length - 2] : (parts.length > 0 ? parts[parts.length - 1] : "unknown");

            String ts = LocalTime.now().format(TIME_FMT);

            JsonElement payloadObj;
            try {
                payloadObj = JsonParser.parseString(payload);
            } catch (Exception e) {
                payloadObj = gson.toJsonTree(payload);
            }

            switch (kind) {
                case "telemetry":
                    JsonElement metrics = payloadObj;
                    if (payloadObj.isJsonObject()) {
                        JsonObject obj = payloadObj.getAsJsonObject();
                        if (obj.has("metrics")) {
                            metrics = obj.get("metrics");
                        }
                    }
                    System.out.printf("📊 [%s] %s telemetry: %s%n", ts, device, metrics.toString());
                    break;

                case "status":
                    String online = "?";
                    if (payloadObj.isJsonObject()) {
                        JsonObject obj = payloadObj.getAsJsonObject();
                        if (obj.has("online")) {
                            online = obj.get("online").toString();
                        }
                    }
                    System.out.printf("🔌 [%s] %s status: %s%n", ts, device, online);
                    break;

                case "event":
                    System.out.printf("⚡ [%s] %s event: %s%n", ts, device, payloadObj.toString());
                    break;

                case "log":
                    System.out.printf("📝 [%s] %s log: %s%n", ts, device, payloadObj.toString());
                    break;

                case "ota":
                    System.out.printf("📦 [%s] %s OTA: %s%n", ts, device, payloadObj.toString());
                    break;

                default:
                    System.out.printf("📨 [%s] %s/%s: %s%n", ts, device, kind, payloadObj.toString());
                    break;
            }
        }

        void stop() {
            try {
                if (client != null && client.isConnected()) {
                    client.disconnect();
                    System.out.printf("[%s] MQTT disconnected%n", LocalTime.now().format(TIME_FMT));
                }
            } catch (MqttException e) {
                System.err.printf("Disconnect error: %s%n", e.getMessage());
            }
        }
    }

    // ─── Main ────────────────────────────────────────────────────────────────

    public static void main(String[] args) throws Exception {
        String platformUrl = "http://localhost:8080";
        String token = null;

        for (int i = 0; i < args.length; i++) {
            switch (args[i]) {
                case "--platform":
                    platformUrl = args[++i];
                    break;
                case "--token":
                    token = args[++i];
                    break;
            }
        }

        if (token == null) {
            System.err.println("--token is required (admin JWT)");
            System.exit(1);
        }

        // Register
        RegistrationResponse config = register(platformUrl, token);

        // Create plugin
        MQTTLoggerPlugin plugin = new MQTTLoggerPlugin(config);

        // Graceful shutdown
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutting down...");
            plugin.stop();
        }));

        // Connect and wait
        plugin.connect();

        // Keep main thread alive
        Thread.currentThread().join();
    }
}
