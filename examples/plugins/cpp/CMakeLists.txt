cmake_minimum_required(VERSION 3.14)
project(plugin_examples LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Find common dependencies
find_package(nlohmann_json REQUIRED)
find_package(CURL REQUIRED)

# ─── Webhook Example (cpp-httplib) ───────────────────────────────────────────

# Download cpp-httplib single header if not found
include(FetchContent)
FetchContent_Declare(
    httplib
    GIT_REPOSITORY https://github.com/yhirose/cpp-httplib.git
    GIT_TAG v0.15.3
)
FetchContent_MakeAvailable(httplib)

add_executable(telemetry_logger telemetry_logger.cpp)
target_link_libraries(telemetry_logger
    PRIVATE
        nlohmann_json::nlohmann_json
        CURL::libcurl
        httplib::httplib
)

# ─── MQTT Example (Eclipse Paho) ─────────────────────────────────────────────

find_package(PahoMqttCpp QUIET)
if(PahoMqttCpp_FOUND)
    add_executable(mqtt_telemetry_logger mqtt_telemetry_logger.cpp)
    target_link_libraries(mqtt_telemetry_logger
        PRIVATE
            PahoMqttCpp::paho-mqttpp3
            nlohmann_json::nlohmann_json
            CURL::libcurl
    )
else()
    message(STATUS "Paho MQTT C++ not found, skipping mqtt_telemetry_logger")
endif()
