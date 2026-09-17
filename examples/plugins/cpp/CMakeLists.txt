cmake_minimum_required(VERSION 3.14)
project(mqtt_plugin_examples LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(nlohmann_json REQUIRED)
find_package(CURL REQUIRED)

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
    message(FATAL_ERROR "Paho MQTT C++ not found — install libpaho-mqttpp3-dev")
endif()
