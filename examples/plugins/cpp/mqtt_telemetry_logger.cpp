/**
 * MQTT Telemetry Logger — External Plugin (Node-RED style)
 *
 * Connects to the platform MQTT broker directly, subscribes to rules bus events,
 * and logs all telemetry/status/events to console.
 *
 * Usage:
 *   # With admin JWT (registers plugin, gets credentials)
 *   ./mqtt_telemetry_logger --platform http://localhost:8080 --token <admin-jwt>
 *
 *   # With existing credentials
 *   ./mqtt_telemetry_logger --mqtt-user extplg_logger --mqtt-pass <password>
 *
 *   # With environment variables
 *   MQTT_USER=extplg_logger MQTT_PASS=<password> ./mqtt_telemetry_logger
 *
 * Dependencies:
 *   - Eclipse Paho MQTT C++ (https://github.com/eclipse/paho.mqtt.cpp)
 *   - nlohmann/json (https://github.com/nlohmann/json)
 *   - libcurl (for HTTP registration)
 *
 * Build (CMake):
 *   cmake -B build -S .
 *   cmake --build build
 */

#include <iostream>
#include <string>
#include <chrono>
#include <thread>
#include <csignal>
#include <atomic>
#include <sstream>
#include <iomanip>
#include <cstdlib>

#include <mqtt/client.h>
#include <nlohmann/json.hpp>
#include <curl/curl.h>

using json = nlohmann::json;

// ─── Configuration ───────────────────────────────────────────────────────────

const std::string PLUGIN_ID = "mqtt-telemetry-logger";
const std::string PLUGIN_NAME = "MQTT Telemetry Logger";
const std::string PLUGIN_DESC = "Subscribes to rules bus via MQTT and logs all device telemetry";

// ─── Globals ─────────────────────────────────────────────────────────────────

std::atomic<bool> g_running{true};

// ─── Helpers ─────────────────────────────────────────────────────────────────

std::string get_timestamp() {
    auto now = std::chrono::system_clock::now();
    auto time = std::chrono::system_clock::to_time_t(now);
    auto tm = std::localtime(&time);
    std::ostringstream oss;
    oss << std::put_time(tm, "%H:%M:%S");
    return oss.str();
}

std::string get_env(const std::string& key, const std::string& def = "") {
    const char* val = std::getenv(key.c_str());
    return val ? std::string(val) : def;
}

// ─── Config ──────────────────────────────────────────────────────────────────

struct PluginConfig {
    std::string mqtt_user;
    std::string mqtt_pass;
    std::string mqtt_host;
    int mqtt_port;
    std::string rules_bus;
};

// ─── Registration ────────────────────────────────────────────────────────────

static size_t write_callback(void* contents, size_t size, size_t nmemb, void* userp) {
    size_t total = size * nmemb;
    static_cast<std::string*>(userp)->append(static_cast<char*>(contents), total);
    return total;
}

PluginConfig register_plugin(const std::string& platform_url, const std::string& jwt_token) {
    json payload = {
        {"id", PLUGIN_ID},
        {"name", PLUGIN_NAME},
        {"description", PLUGIN_DESC}
    };

    std::string url = platform_url + "/api/v1/plugins/external";
    std::string response_body;

    CURL* curl = curl_easy_init();
    if (!curl) {
        throw std::runtime_error("Failed to initialize curl");
    }

    struct curl_slist* headers = nullptr;
    headers = curl_slist_append(headers, "Content-Type: application/json");
    headers = curl_slist_append(headers, ("Authorization: Bearer " + jwt_token).c_str());

    curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, payload.dump().c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response_body);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 10L);

    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);
        throw std::runtime_error(std::string("HTTP request failed: ") + curl_easy_strerror(res));
    }

    long http_code = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);

    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);

    if (http_code != 201) {
        throw std::runtime_error("Registration failed: HTTP " + std::to_string(http_code));
    }

    json resp = json::parse(response_body);
    PluginConfig cfg;
    cfg.mqtt_user = resp.value("mqtt_user", "");
    cfg.mqtt_pass = resp.value("mqtt_pass", "");
    cfg.mqtt_host = resp.value("mqtt_host", "localhost");
    cfg.mqtt_port = resp.value("mqtt_port", 1883);
    cfg.rules_bus = resp.value("rules_bus", "platform/v1");

    std::cout << "[" << get_timestamp() << "] Registered: mqtt_user=" << cfg.mqtt_user << std::endl;

    return cfg;
}

// ─── MQTT Client ─────────────────────────────────────────────────────────────

class MQTTLoggerPlugin : public virtual mqtt::callback {
private:
    mqtt::async_client client_;
    PluginConfig config_;

public:
    MQTTLoggerPlugin(const PluginConfig& config)
        : client_("tcp://" + config.mqtt_host + ":" + std::to_string(config.mqtt_port),
                  "extplg_" + PLUGIN_ID,
                  mqtt::create_options_ptr(mqtt::CREATE_OPTIONS_VERSION))
        , config_(config)
    {
        client_.set_callback(*this);
    }

    void connect() {
        mqtt::connect_options opts;
        opts.set_user_name(config_.mqtt_user);
        opts.set_password(config_.mqtt_pass);
        opts.set_keep_alive_interval(60);
        opts.set_clean_session(true);
        opts.set_automatic_reconnect(true);
        opts.set_connect_timeout(10);

        std::cout << "[" << get_timestamp() << "] Connecting to MQTT broker tcp://"
                  << config_.mqtt_host << ":" << config_.mqtt_port
                  << " as " << config_.mqtt_user << std::endl;

        client_.connect(opts)->wait();
    }

    void connected(const std::string& cause) override {
        std::cout << "[" << get_timestamp() << "] MQTT connected" << std::endl;

        std::string topic = config_.rules_bus + "/events/#";
        client_.subscribe(topic, 0)->wait();
        std::cout << "[" << get_timestamp() << "] Subscribed to " << topic << std::endl;
    }

    void connection_lost(const std::string& cause) override {
        std::cout << "[" << get_timestamp() << "] MQTT disconnected: " << cause << std::endl;
    }

    void message_arrived(mqtt::const_message_ptr msg) override {
        std::string topic = msg->get_topic();
        std::string payload = msg->to_string();

        std::vector<std::string> parts;
        std::istringstream iss(topic);
        std::string part;
        while (std::getline(iss, part, '/')) {
            parts.push_back(part);
        }

        std::string kind = "unknown";
        std::string device = "unknown";
        if (parts.size() > 3) {
            kind = parts.back();
            device = parts[parts.size() - 2];
        } else if (!parts.empty()) {
            device = parts.back();
        }

        std::string ts = get_timestamp();

        json payload_obj;
        try {
            payload_obj = json::parse(payload);
        } catch (...) {
            payload_obj = payload;
        }

        if (kind == "telemetry") {
            json metrics = payload_obj;
            if (payload_obj.is_object() && payload_obj.contains("metrics")) {
                metrics = payload_obj["metrics"];
            }
            std::cout << "📊 [" << ts << "] " << device << " telemetry: " << metrics.dump() << std::endl;
        }
        else if (kind == "status") {
            std::string online = "?";
            if (payload_obj.is_object() && payload_obj.contains("online")) {
                online = payload_obj["online"].dump();
            }
            std::cout << "🔌 [" << ts << "] " << device << " status: " << online << std::endl;
        }
        else if (kind == "event") {
            std::cout << "⚡ [" << ts << "] " << device << " event: " << payload_obj.dump() << std::endl;
        }
        else if (kind == "log") {
            std::cout << "📝 [" << ts << "] " << device << " log: " << payload_obj.dump() << std::endl;
        }
        else if (kind == "ota") {
            std::cout << "📦 [" << ts << "] " << device << " OTA: " << payload_obj.dump() << std::endl;
        }
        else {
            std::cout << "📨 [" << ts << "] " << device << "/" << kind << ": " << payload_obj.dump() << std::endl;
        }
    }

    void delivery_complete(mqtt::delivery_token_ptr tok) override {}

    void stop() {
        if (client_.is_connected()) {
            client_.disconnect()->wait();
            std::cout << "[" << get_timestamp() << "] MQTT disconnected" << std::endl;
        }
    }
};

// ─── Signal Handler ──────────────────────────────────────────────────────────

void signal_handler(int) {
    g_running = false;
}

// ─── Main ────────────────────────────────────────────────────────────────────

int main(int argc, char* argv[]) {
    std::string platform_url = "http://localhost:8080";
    std::string token;
    std::string mqtt_user;
    std::string mqtt_pass;
    std::string mqtt_host = "localhost";
    int mqtt_port = 1883;
    std::string rules_bus = "platform/v1";

    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        if (arg == "--platform" && i + 1 < argc) {
            platform_url = argv[++i];
        } else if (arg == "--token" && i + 1 < argc) {
            token = argv[++i];
        } else if (arg == "--mqtt-user" && i + 1 < argc) {
            mqtt_user = argv[++i];
        } else if (arg == "--mqtt-pass" && i + 1 < argc) {
            mqtt_pass = argv[++i];
        } else if (arg == "--mqtt-host" && i + 1 < argc) {
            mqtt_host = argv[++i];
        } else if (arg == "--mqtt-port" && i + 1 < argc) {
            mqtt_port = std::stoi(argv[++i]);
        } else if (arg == "--rules-bus" && i + 1 < argc) {
            rules_bus = argv[++i];
        }
    }

    PluginConfig config;

    if (!token.empty()) {
        // Registration mode
        config = register_plugin(platform_url, token);
    } else if (!mqtt_user.empty() && !mqtt_pass.empty()) {
        // Direct credentials from args
        config.mqtt_user = mqtt_user;
        config.mqtt_pass = mqtt_pass;
        config.mqtt_host = mqtt_host;
        config.mqtt_port = mqtt_port;
        config.rules_bus = rules_bus;
        std::cout << "[" << get_timestamp() << "] Using direct credentials for " << mqtt_user << std::endl;
    } else {
        // Try environment variables
        std::string env_user = get_env("MQTT_USER");
        std::string env_pass = get_env("MQTT_PASS");
        if (!env_user.empty() && !env_pass.empty()) {
            config.mqtt_user = env_user;
            config.mqtt_pass = env_pass;
            config.mqtt_host = get_env("MQTT_HOST", mqtt_host);
            std::string port_str = get_env("MQTT_PORT");
            config.mqtt_port = port_str.empty() ? mqtt_port : std::stoi(port_str);
            config.rules_bus = get_env("RULES_BUS", rules_bus);
            std::cout << "[" << get_timestamp() << "] Using credentials from environment for " << env_user << std::endl;
        } else {
            std::cerr << "Either --token or --mqtt-user/--mqtt-pass (or env vars) required" << std::endl;
            return 1;
        }
    }

    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);

    try {
        curl_global_init(CURL_GLOBAL_DEFAULT);

        MQTTLoggerPlugin plugin(config);
        plugin.connect();

        while (g_running) {
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }

        std::cout << "Shutting down..." << std::endl;
        plugin.stop();

        curl_global_cleanup();
        return 0;

    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        curl_global_cleanup();
        return 1;
    }
}
