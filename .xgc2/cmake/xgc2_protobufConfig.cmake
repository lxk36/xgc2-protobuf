get_filename_component(
  XGC2_PROTOBUF_PREFIX
  "${CMAKE_CURRENT_LIST_DIR}/../../.."
  ABSOLUTE
)

set(XGC2_PROTOBUF_SCHEMA_ROOT "${XGC2_PROTOBUF_PREFIX}/share/xgc2-protobuf")
set(XGC2_PROTOBUF_PROTO_ROOT "${XGC2_PROTOBUF_SCHEMA_ROOT}/proto")
set(XGC2_PROTOBUF_DESCRIPTOR_SET
    "${XGC2_PROTOBUF_SCHEMA_ROOT}/descriptors/xgc2-protocols.pb")
set(XGC2_PROTOBUF_REGISTRY_JSON "${XGC2_PROTOBUF_SCHEMA_ROOT}/registry/registry.json")
set(XGC2_PROTOBUF_REGISTRY_YAML "${XGC2_PROTOBUF_SCHEMA_ROOT}/registry/messages.yaml")
set(XGC2_PROTOBUF_PROFILES_DIR "${XGC2_PROTOBUF_SCHEMA_ROOT}/profiles")
set(XGC2_PROTOBUF_PROFILE_REGISTRY "${XGC2_PROTOBUF_PROFILES_DIR}/registry.json")

foreach(_xgc2_protobuf_path IN ITEMS
    "${XGC2_PROTOBUF_PROTO_ROOT}"
    "${XGC2_PROTOBUF_DESCRIPTOR_SET}"
    "${XGC2_PROTOBUF_REGISTRY_JSON}"
    "${XGC2_PROTOBUF_REGISTRY_YAML}"
    "${XGC2_PROTOBUF_PROFILES_DIR}"
    "${XGC2_PROTOBUF_PROFILE_REGISTRY}")
  if(NOT EXISTS "${_xgc2_protobuf_path}")
    set(xgc2_protobuf_FOUND FALSE)
    set(xgc2_protobuf_NOT_FOUND_MESSAGE
        "xgc2-protobuf installation is incomplete: ${_xgc2_protobuf_path} is missing")
    return()
  endif()
endforeach()

if(NOT TARGET xgc2_protobuf::schemas)
  add_library(xgc2_protobuf::schemas INTERFACE IMPORTED)
endif()

set(xgc2_protobuf_FOUND TRUE)
unset(_xgc2_protobuf_path)
