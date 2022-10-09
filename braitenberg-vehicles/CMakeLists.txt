cmake_minimum_required(VERSION 2.6 FATAL_ERROR)

project(braitenberg_vehicles)

if(CMAKE_COMPILER_IS_GNUCXX)
  add_definitions(-std=c++11)
endif()

set(Boost_USE_STATIC_RUNTIME OFF)
find_package(Boost 1.49 COMPONENTS program_options REQUIRED)
include_directories(${Boost_INCLUDE_DIRS})

find_package(GLUT REQUIRED)
include_directories(${GLUT_INCLUDE_DIR})

find_package(OpenGL REQUIRED)
include_directories(${OPENGL_INCLUDE_DIR})

find_package(Armadillo REQUIRED)
include_directories(${Armadillo_INCLUDE_DIR})

add_executable(
  braitenberg
  braitenberg.cc
  light_display.cc
  vehicle_display.cc
  orthographic_viewport.cc
  light_model.cc
  vehicle_model.cc
  )
target_link_libraries(
  braitenberg
  ${Boost_LIBRARIES}
  ${GLUT_LIBRARIES}
  ${OPENGL_LIBRARIES}
  ${Armadillo_LIBRARIES}
  )

configure_file(
  ${CMAKE_CURRENT_SOURCE_DIR}/example_config.cfg
  ${CMAKE_CURRENT_BINARY_DIR}/example_config.cfg
)