/**
 * Telemetry Logger — Webhook External Plugin
 *
 * Receives telemetry events from the platform via HTTP POST (JSON-RPC)
 * and logs them to console. Sends periodic heartbeats.
 *
 * Usage:
 *   ./telemetry_logger --platform http://localhost:8080 --token <admin-jwt> --port 9000
 *
 * Dependencies:
 *   - nlohmann/json (https://github.com/nlohmann/json)
 *   - cpp-httplib (https://github.com/yhirose/cpp-httplib) — single header
 *   - libcurl (for HTTP registration and heartbeat)
 *
 * Build (CMake):
 *   cmake -B build -S .
 *   cmake --build build
 *
 * The plugin:
 * 1. Registers with the platform using admin JWT
 * 2. Starts HTTP server on specified port
 * 3. Receives plugin.on_event, plugin.apply_config, plugin.detach via JSON-RPC
 * 4. Sends heartbeat every 30 seconds
 */

#include <iostream>
#include <string>
#include <chrono>
#include <thread>
#include <csignal>
#include <atomic>
#include <sstream>
#include <iomanip>
#include <mutex>

#include <nlohmann/json.hpp>
#include <httplib.h>
#include <curl/curl.h>

using json = nlohmann::json;

// ─── Configuration ───────────────────────────────────────────────────────────

const std::string PLUGIN_ID = "telemetry-logger";
const std::string PLUGIN_NAME = "Telemetry Logger";
const std::string PLUGIN_DESC = "Logs device telemetry to console via webhook";

// ─── Globals ─────────────────────────────────────────────────────────────────

std::string g_plugin_token;
std::string g_platform_url;
std::atomic<int64_t> g_events_count{0};
auto g_start_time = std::chrono::steady_clock::now();
std::atomic<bool> g_running{true};
std::mutex g_mutex;

// ─── Helpers ─────────────────────────────────────────────────────────────────

std::string get_timestamp() {
    auto now = std::chrono::system_clock::now();
    auto time = std::chrono::system_clock::to_time_t(now);
    auto tm = std::localtime(&time);
    std::ostringstream oss;
    oss << std::put_time(tm, "%H:%M:%S");
    return oss.str();
}

static size_t write_callback(void* contents, size_t size, size_t nmemb, void* userp) {
    size_t total = size * nmemb;
    static_cast<std::string*>(userp)->append(static_cast<char*>(contents), total);
    return total;
}

// ─── Registration ────────────────────────────────────────────────────────────

std::string register_plugin(const std::string& platform_url, const std::string& jwt_token, const std::string& endpoint) {
    json payload = {
        {"id", PLUGIN_ID},
        {"name", PLUGIN_NAME},
        {"description", PLUGIN_DESC},
        {"endpoint", endpoint},
        {"auto", false},
        {"interval_ms", 0},
        {"default_config", json::object()},
        {"form", json::array()}
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
    std::string token = resp.value("token", "");
    std::string id = resp.value("id", "");

    std::cout << "[" << get_timestamp() << "] Registered: id=" << id
              << " token=" << token.substr(0, 16) << "..." << std::endl;

    return token;
}

// ─── Heartbeat ───────────────────────────────────────────────────────────────

void heartbeat_loop() {
    while (g_running) {
        std::this_thread::sleep_for(std::chrono::seconds(30));
        if (!g_running) break;

        json payload = {
            {"jsonrpc", "2.0"},
            {"id", 1},
            {"method", "platform.heartbeat"},
            {"params", {
                {"plugin_id", PLUGIN_ID},
                {"status", "ok"},
                {"stats", {
                    {"events_processed", g_events_count.load()},
                    {"uptime_sec", std::chrono::duration_cast<std::chrono::seconds>(
                        std::chrono::steady_clock::now() - g_start_time).count()}
                }}
            }}
        };

        std::string url = g_platform_url + "/api/plugins/external/rpc";
        std::string response_body;

        CURL* curl = curl_easy_init();
        if (!curl) continue;

        struct curl_slist* headers = nullptr;
        headers = curl_slist_append(headers, "Content-Type: application/json");
        headers = curl_slist_append(headers, ("Authorization: Bearer " + g_plugin_token).c_str());

        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, payload.dump().c_str());
        curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_callback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response_body);
        curl_easy_setopt(curl, CURLOPT_TIMEOUT, 5L);

        CURLcode res = curl_easy_perform(curl);

        long http_code = 0;
        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);

        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);

        if (res == CURLE_OK && http_code == 200) {
            std::cout << "[" << get_timestamp() << "] Heartbeat OK (events: "
                      << g_events_count.load() << ")" << std::endl;
        } else {
            std::cout << "[" << get_timestamp() << "] Heartbeat failed" << std::endl;
        }
    }
}

// ─── JSON-RPC Helpers ────────────────────────────────────────────────────────

std::string make_result(const json& id, const json& result) {
    json resp = {{"jsonrpc", "2.0"}, {"id", id}, {"result", result}};
    return resp.dump();
}

std::string make_error(const json& id, int code, const std::string& message) {
    json resp = {{"jsonrpc", "2.0"}, {"id", id}, {"error", {{"code", code}, {"message", message}}}};
    return resp.dump();
}

// ─── HTTP Handler ────────────────────────────────────────────────────────────

void handle_rpc(const httplib::Request& req, httplib::Response& res) {
    json req_json;
    try {
        req_json = json::parse(req.body);
    } catch (...) {
        res.set_content(make_error(nullptr, -32700, "parse error"), "application/json");
        return;
    }

    std::string method = req_json.value("method", "");
    json id = req_json.value("id", nullptr);
    json params = req_json.value("params", json::object());

    std::string response;

    if (method == "plugin.on_event") {
        g_events_count++;

        int64_t binding_id = params.value("binding_id", 0);
        int64_t device_pk = params.value("device_pk", 0);
        std::string kind = params.value("kind", "unknown");
        json payload = params.value("payload", json::object());

        std::string ts = get_timestamp();

        if (kind == "telemetry") {
            std::cout << "📊 Device #" << device_pk << " (binding " << binding_id << "):" << std::endl;
            if (payload.is_object()) {
                for (auto& [key, value] : payload.items()) {
                    std::cout << "   " << key << ": " << value << std::endl;
                }
            }
        } else if (kind == "status") {
            bool online = params.value("online", false);
            std::cout << "🔌 Device #" << device_pk << ": " << (online ? "online" : "offline") << std::endl;
        } else if (kind == "event") {
            std::cout << "⚡ Device #" << device_pk << " event: " << payload.dump() << std::endl;
        } else {
            std::cout << "📨 Device #" << device_pk << " [" << kind << "]: " << payload.dump() << std::endl;
        }

        response = make_result(id, {{"ok", true}});
    }
    else if (method == "plugin.apply_config") {
        int64_t binding_id = params.value("binding_id", 0);
        std::cout << "Binding " << binding_id << ": config applied" << std::endl;
        response = make_result(id, {{"ok", true}});
    }
    else if (method == "plugin.detach") {
        int64_t binding_id = params.value("binding_id", 0);
        std::cout << "Binding " << binding_id << ": detached" << std::endl;
        response = make_result(id, {{"ok", true}});
    }
    else {
        response = make_error(id, -32601, "unknown method: " + method);
    }

    res.set_content(response, "application/json");
}

// ─── Signal Handler ──────────────────────────────────────────────────────────

void signal_handler(int) {
    g_running = false;
}

// ─── Main ────────────────────────────────────────────────────────────────────

int main(int argc, char* argv[]) {
    std::string platform_url = "http://localhost:8080";
    std::string token;
    int port = 9000;

    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        if (arg == "--platform" && i + 1 < argc) {
            platform_url = argv[++i];
        } else if (arg == "--token" && i + 1 < argc) {
            token = argv[++i];
        } else if (arg == "--port" && i + 1 < argc) {
            port = std::stoi(argv[++i]);
        }
    }

    if (token.empty()) {
        std::cerr << "--token is required (admin JWT)" << std::endl;
        return 1;
    }

    g_platform_url = platform_url;

    std::cout << "==================================================" << std::endl;
    std::cout << "Telemetry Logger Plugin (Webhook)" << std::endl;
    std::cout << "==================================================" << std::endl;

    // Setup signal handlers
    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);

    try {
        // Initialize curl
        curl_global_init(CURL_GLOBAL_DEFAULT);

        // Determine endpoint
        char hostname[256];
        gethostname(hostname, sizeof(hostname));
        std::string endpoint = "http://" + std::string(hostname) + ":" + std::to_string(port) + "/rpc";

        // Register
        g_plugin_token = register_plugin(platform_url, token, endpoint);

        // Start heartbeat thread
        std::thread heartbeat_thread(heartbeat_loop);

        // Start HTTP server
        httplib::Server svr;
        svr.Post("/rpc", handle_rpc);

        std::cout << "Listening on port " << port << "..." << std::endl;
        std::cout << "Waiting for events from platform..." << std::endl;
        std::cout << "--------------------------------------------------" << std::endl;

        // Run server in a thread so we can check g_running
        std::thread server_thread([&svr, port]() {
            svr.listen("0.0.0.0", port);
        });

        // Wait for signal
        while (g_running) {
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }

        std::cout << "Shutting down..." << std::endl;
        svr.stop();
        server_thread.join();
        heartbeat_thread.join();

        curl_global_cleanup();
        return 0;

    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        curl_global_cleanup();
        return 1;
    }
}
