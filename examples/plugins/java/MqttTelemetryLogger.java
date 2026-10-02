/**
 * MQTT Telemetry Logger — External Plugin (Node-RED style)
 *
 * Connects to the platform MQTT broker directly, subscribes to rules bus events,
 * and logs all telemetry/status/events to console.
 *
 * Usage:
 *   # With admin JWT (registers plugin, gets credentials)
 *   java -jar target/mqtt-telemetry-logger-1.0.0.jar --platform http://localhost:8080 --token <admin-jwt>
 *
 *   # With existing credentials
 *   java -jar target/mqtt-telemetry-logger-1.0.0.jar --mqtt-user extplg_acme_mqtt-telemetry-logger --mqtt-pass <password>
 *
 *   # With environment variables
 *   MQTT_USER=extplg_acme_mqtt-telemetry-logger MQTT_PASS=<password> java -jar target/mqtt-telemetry-logger-1.0.0.jar
 *
 * Dependencies (Maven):
 *   - org.eclipse.paho:org.eclipse.paho.client.mqttv3:1.2.5
 *   - com.google.code.gson:gson:2.10.1
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

public class MqttTelemetryLogger {

    // ─── Configuration ───────────────────────────────────────────────────────

    private static final String PLUGIN_ID = "mqtt-telemetry-logger";
    private static final String PLUGIN_NAME = "MQTT Telemetry Logger";
    private static final String PLUGIN_DESC = "Subscribes to rules bus via MQTT and logs all device telemetry";
    private static final DateTimeFormatter TIME_FMT = DateTimeFormatter.ofPattern("HH:mm:ss");

    // ─── Config ──────────────────────────────────────────────────────────────

    static class PluginConfig {
        String mqttUser;
        String mqttPass;
        String mqttHost;
        int mqttPort;
        String rulesBus;
    }

    // ─── Registration ────────────────────────────────────────────────────────

    static class RegistrationResponse {
        boolean ok;
        String id;
        String mqtt_user;
        String mqtt_pass;
        String mqtt_host;
        int mqtt_port;
        String rules_bus;
    }

    static PluginConfig register(String platformUrl, String jwtToken) throws Exception {
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

        PluginConfig cfg = new PluginConfig();
        cfg.mqttUser = resp.mqtt_user;
        cfg.mqttPass = resp.mqtt_pass;
        cfg.mqttHost = resp.mqtt_host;
        cfg.mqttPort = resp.mqtt_port;
        cfg.rulesBus = resp.rules_bus;
        return cfg;
    }

    // ─── MQTT Client ─────────────────────────────────────────────────────────

    static class MQTTLoggerPlugin {
        private final PluginConfig config;
        private final String eventsRoot; // {rules_bus}/events/{account_slug}/
        private MqttClient client;
        private final Gson gson = new Gson();

        MQTTLoggerPlugin(PluginConfig config) {
            this.config = config;
            // The ACL only allows subscribing inside the plugin's own account.
            this.eventsRoot = config.rulesBus + "/events/" + accountSlug(config.mqttUser) + "/";
        }

        /** extplg_acme_mqtt-telemetry-logger -> acme */
        static String accountSlug(String mqttUser) {
            String prefix = "extplg_";
            int sep = mqttUser.indexOf('_', prefix.length());
            if (!mqttUser.startsWith(prefix) || sep <= prefix.length()) {
                throw new IllegalArgumentException("not an external plugin user: " + mqttUser);
            }
            return mqttUser.substring(prefix.length(), sep);
        }

        void connect() throws Exception {
            String clientId = "extplg_" + PLUGIN_ID;
            String broker = String.format("tcp://%s:%d", config.mqttHost, config.mqttPort);

            client = new MqttClient(broker, clientId, new MemoryPersistence());

            MqttConnectOptions opts = new MqttConnectOptions();
            opts.setUserName(config.mqttUser);
            opts.setPassword(config.mqttPass.toCharArray());
            opts.setKeepAliveInterval(60);
            opts.setCleanSession(true);
            opts.setAutomaticReconnect(true);
            opts.setConnectionTimeout(10);

            client.setCallback(new MqttCallbackExtended() {
                @Override
                public void connectComplete(boolean reconnect, String serverURI) {
                    System.out.printf("[%s] MQTT connected%s%n",
                            LocalTime.now().format(TIME_FMT), reconnect ? " (reconnect)" : "");

                    String topic = eventsRoot + "#";
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
                    LocalTime.now().format(TIME_FMT), broker, config.mqttUser);
            client.connect(opts);
        }

        void handleMessage(String topic, String payload) {
            // {product}/{device}/{kind}; kind may contain a slash (rpc/result, attributes/reported).
            String rest = topic.startsWith(eventsRoot) ? topic.substring(eventsRoot.length()) : topic;
            String[] parts = rest.split("/", 3);
            if (parts.length < 3) {
                System.err.printf("Unexpected topic: %s%n", topic);
                return;
            }
            String device = parts[0] + "/" + parts[1];
            String kind = parts[2];

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
                        if (obj.has("keys")) {
                            metrics = obj.get("keys");
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
        String mqttUser = null;
        String mqttPass = null;
        String mqttHost = "localhost";
        int mqttPort = 1883;
        String rulesBus = "platform/v1";

        for (int i = 0; i < args.length; i++) {
            switch (args[i]) {
                case "--platform":
                    platformUrl = args[++i];
                    break;
                case "--token":
                    token = args[++i];
                    break;
                case "--mqtt-user":
                    mqttUser = args[++i];
                    break;
                case "--mqtt-pass":
                    mqttPass = args[++i];
                    break;
                case "--mqtt-host":
                    mqttHost = args[++i];
                    break;
                case "--mqtt-port":
                    mqttPort = Integer.parseInt(args[++i]);
                    break;
                case "--rules-bus":
                    rulesBus = args[++i];
                    break;
            }
        }

        PluginConfig config;

        if (token != null) {
            // Registration mode
            config = register(platformUrl, token);
        } else if (mqttUser != null && mqttPass != null) {
            // Direct credentials from args
            config = new PluginConfig();
            config.mqttUser = mqttUser;
            config.mqttPass = mqttPass;
            config.mqttHost = mqttHost;
            config.mqttPort = mqttPort;
            config.rulesBus = rulesBus;
            System.out.printf("[%s] Using direct credentials for %s%n",
                    LocalTime.now().format(TIME_FMT), mqttUser);
        } else if (System.getenv("MQTT_USER") != null && System.getenv("MQTT_PASS") != null) {
            // Direct credentials from env
            config = new PluginConfig();
            config.mqttUser = System.getenv("MQTT_USER");
            config.mqttPass = System.getenv("MQTT_PASS");
            config.mqttHost = System.getenv("MQTT_HOST") != null ? System.getenv("MQTT_HOST") : mqttHost;
            config.mqttPort = System.getenv("MQTT_PORT") != null ? Integer.parseInt(System.getenv("MQTT_PORT")) : mqttPort;
            config.rulesBus = System.getenv("RULES_BUS") != null ? System.getenv("RULES_BUS") : rulesBus;
            System.out.printf("[%s] Using credentials from environment for %s%n",
                    LocalTime.now().format(TIME_FMT), config.mqttUser);
        } else {
            System.err.println("Either --token or --mqtt-user/--mqtt-pass (or env vars) required");
            System.exit(1);
            return;
        }

        MQTTLoggerPlugin plugin = new MQTTLoggerPlugin(config);

        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutting down...");
            plugin.stop();
        }));

        plugin.connect();
        Thread.currentThread().join();
    }
}
