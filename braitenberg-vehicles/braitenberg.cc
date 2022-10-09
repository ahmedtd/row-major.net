
#include <algorithm>
using std::generate;
using std::transform;
using std::min;
#include <chrono>
using std::chrono::system_clock;
using std::chrono::duration;
using std::chrono::milliseconds;
using std::chrono::duration_cast;
#include <fstream>
using std::ifstream;
#include <iostream>
using std::cout;
using std::cerr;
using std::endl;
#include <random>
using std::mt19937_64;
using std::normal_distribution;
using std::uniform_real_distribution;
#include <string>
using std::string;
#include <vector>
using std::vector;
using std::move;

#include <boost/program_options.hpp>
namespace po = boost::program_options;

#include <armadillo>
using arma::vec;
using arma::datum;

#include <GL/freeglut.h>

#include "orthographic_viewport.hh"
#include "light_display.hh"
#include "vehicle_display.hh"
#include "light_model.hh"
#include "vehicle_model.hh"

void display_callback();
void reshape_callback(int new_w, int new_h);
void keyboard_callback(unsigned char key, int xpix, int ypix);

orthographic_viewport *gp_viewport;

vector<light_display> light_display_state;
vector<vehicle_display> vehicle_display_state;

int main(int argc, char **argv)
{
    // Initialize glut
    glutInit(&argc, argv);
    glutInitDisplayMode(GLUT_RGBA | GLUT_DOUBLE | GLUT_ALPHA);

    // Parse options

    po::options_description desc("Allowed options");

    desc.add_options()
        ("help", "Print this help text")
        ("add-light",
         po::value< vector<light_model> >()->composing(),
         "Add a light to the simulation"
        )
        ("random-lights",
         po::value<int>()->composing(),
         "Add some uniformly-distributed lights to the environment"
        )
        ("add-vehicle",
         po::value< vector<vehicle_model> >()->composing(),
         "Add a vehicle to the simulation"
        )
        ("random-vehicles",
         po::value<int>()->composing(),
         "Add some uniformly-distributed vehicles to the environment"
        )
        ("file",
         po::value<string>(),
         "A configuration file to load."
        );
    
    po::variables_map vm;
    po::store(po::parse_command_line(argc, argv, desc), vm);

    po::notify(vm);

    // Load options from config file if instructed
    if(vm.count("file"))
    {
        ifstream configstream(vm["file"].as<string>());
        
        if(configstream)
        {
            po::store(
                po::parse_config_file(
                    configstream,
                    desc
                ),
                vm
            );
            
            po::notify(vm);
        }
        else
        {
            cerr << "Specified configuration file does not exist" << endl;
            return 1;
        }
    }

    if(vm.count("help"))
    {
        cerr << desc < "\n";
        return 1;
    }
    
    // More GLUT setup
   
    // Create a window, automatically set as the current window
    int win_handle = glutCreateWindow("Team 2: Braitenberg");
 
    glClearColor(0.0, 0.0, 0.0, 1.0);
    
    glEnable(GL_BLEND);
    glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA);

    glutDisplayFunc(display_callback);
    glutReshapeFunc(reshape_callback);
    glutKeyboardFunc(keyboard_callback);

    // Setup the orthographic_viewport
    vec cur_win_size; cur_win_size << 100 << 100;
    vec origin; origin << 0 << 0;
    vec x_basis; x_basis << 1 << 0; // x basis in screen space
    gp_viewport = new orthographic_viewport(
        cur_win_size,
        200,
        origin,
        x_basis
    );

    // Create a random number source
    mt19937_64 gen(12345);

    // Load the intial simulation state

    // Load lights
    vector<light_model> specified_lights;

    // Explicitly-specified lights
    if(vm.count("add-light"))
    {
        specified_lights = vm["add-light"].as<vector<light_model> >();
    }
    
    // Randomly-generated lights
    if(vm.count("random-lights"))
    {
        // Create a normal distribution
        normal_distribution<> dist(
            0,
            2 * sqrt(vm["random-lights"].as<int>())
        );

        uniform_real_distribution<> intens_dist(1, 5);

        // Ensure space for the newly generated sequence.
        vector<light_model>::size_type old_size = specified_lights.size();
        specified_lights.resize(
            specified_lights.size() + vm["random-lights"].as<int>()
        );

        auto generate_light = [&gen, &dist, &intens_dist]() {         
            vec position;
            position << dist(gen) << dist(gen);
            
            return light_model(
                position,
                intens_dist(gen)
            );
        };

        generate(
            begin(specified_lights) + old_size,
            end(specified_lights),
            generate_light
        );
    }
    
    // Put our lights into an environment
    light_environment_model lights(move(specified_lights));
    
    // Generate the display halves of our lights.
    light_display_state = lights.gen_display();

    // Populate the environment with vehicles
    vector<vehicle_model> vehicles;

    // Explicitly - specified vehicles
    if(vm.count("add-vehicle"))
    {
        vehicles = vm["add-vehicle"].as<vector<vehicle_model> >();
    }

    // Randomly-generated vehicles
    if(vm.count("random-vehicles"))
    {
        // Create a normal distribution for position
        normal_distribution<> pos_dist(
            0,
            1.5 * sqrt(vm["random-vehicles"].as<int>())
        );

        // Create a uniform distribution for orientation
        uniform_real_distribution<> orient_dist(0, 2 * datum::pi);
        
        // Create a uniform distribution for velocity values
        uniform_real_distribution<> connect_dist(0, 3);

        // Reserve space for the randomly-generated vehicles
        vector<vehicle_model>::size_type old_size = vehicles.size();
        vehicles.resize(vehicles.size() + vm["random-vehicles"].as<int>());

        auto generate_vehicle
            = [&gen, &pos_dist, &orient_dist, &connect_dist]() {

            vec position;
            position << pos_dist(gen) << pos_dist(gen);
            
            return vehicle_model(
                position,
                orient_dist(gen),
                connect_dist(gen),
                vehicle_type::follower
            );
        };

        generate(
            begin(vehicles) + old_size,
            end(vehicles),
            generate_vehicle
        );
    }

    // Track which vehicle vector we're working from;
    int side = 0;
    vector<vehicle_model> other_vehicles = vehicles;

    // Generate the display halves of our vehicles
    vehicle_display_state.resize(vehicles.size());
    auto gen_vehicle_display = [](const vehicle_model &vehicle) {
        return vehicle.gen_display();
    };
    transform(
        begin(vehicles),
        end(vehicles),
        begin(vehicle_display_state),
        gen_vehicle_display
    );

    // Enter main loop
    system_clock::time_point last_draw = system_clock::now();
    system_clock::time_point last_calc = last_draw;
    system_clock::duration draw_elapsed_simulated = milliseconds(0);
    while(true)
    {
        glutMainLoopEvent();

        // Get elapsed time since last calc
        system_clock::time_point now = system_clock::now();
        
        // Slow time if we are taking longer than a deadline per calculation
        // pass.
        system_clock::duration calc_elapsed_real = now - last_calc;
        
        system_clock::duration calc_elapsed_simulated
            = (calc_elapsed_real > milliseconds(10))
            ? milliseconds(10)
            : calc_elapsed_real;
        
        // Increment the count of time that has been simulated since the last
        // draw.
        draw_elapsed_simulated += calc_elapsed_simulated;
        
        double calc_elapsed = duration<double>(calc_elapsed_simulated).count();

        // Update the vehicles
        if(side == 0)
        {
            for(auto &vehicle : vehicles)
            {
                vehicle.evolve(calc_elapsed, lights, other_vehicles);
            }
            
        }
        else
        {
            for(auto &vehicle : other_vehicles)
            {
                vehicle.evolve(calc_elapsed, lights, vehicles);
            }
        }
        
        // Check if we need to render.  The check is done on real time elapsed
        // since the last render call.
        if((now - last_draw) > milliseconds(15))
        {
            // We calculate the elapsed time based on the amount of time that
            // has been simulated since last draw.  This allows animations to
            // slow when time is slowed.
            double draw_elapsed
                = duration<double>(draw_elapsed_simulated).count();
            draw_elapsed_simulated = milliseconds(0);

            // Call into the display update code.  The model's update of the
            // display is meant to be a fast call.
            lights.update_display(
                light_display_state,
                draw_elapsed
            );
            
            auto vehicle_it = begin(vehicles);
            auto vehicle_end = end(vehicles);
            if(side == 1)
            {
                vehicle_it = begin(other_vehicles);
                vehicle_end = end(other_vehicles);
            }
            

            auto vehicle_display_it = begin(vehicle_display_state);
            for(; vehicle_it != vehicle_end;
                ++vehicle_it, ++vehicle_display_it)
            {
                (*vehicle_it).update_display(
                    *vehicle_display_it,
                    draw_elapsed
                );
            }

            glutPostRedisplay();
            
            last_draw = now;
        }

        if(side == 0)
        {
            other_vehicles = vehicles;
            side = 1;
        }
        else
        {
            vehicles = other_vehicles;
            side = 0;
        }

        last_calc = now;
    }

    delete gp_viewport;
}

void display_callback()
{
    glClear(GL_COLOR_BUFFER_BIT);
 
    glMatrixMode(GL_MODELVIEW);
    glLoadIdentity();
   
    // Draw the grid
    gp_viewport->draw_cartesian_grid();

    // Draw what must be drawn
    for(auto &light : light_display_state)
    {
        light.render();
    }

    for(const auto &vehicle : vehicle_display_state)
    {
        vehicle.render();
    }

    glutSwapBuffers();
}

void reshape_callback(int new_w, int new_h)
{
    glViewport(0, 0, new_w, new_h);

    vec new_size;
    new_size << new_w << new_h;

    gp_viewport->screen_size(new_size);
    gp_viewport->use();

    glutPostRedisplay();
}

void keyboard_callback(unsigned char key, int xpix, int ypix)
{
    double pixels_per_meter = gp_viewport->pixels_per_meter();
    
    vec translate(2);
    translate(0) = 0;
    translate(1) = 0;

    vec step = gp_viewport->screen_size();
    step = step / 2 / pixels_per_meter;

    if(key=='i')
    {
        pixels_per_meter *= 2.0;
    }
    else if(key=='o')
    {
        pixels_per_meter /= 2.0;
    }
    else if(key=='w')
    {
        translate(1) = step(1); 
    }
    else if(key=='s')
    {
        translate(1) = -step(1);
    }
    else if(key=='a')
    {
        translate(0) = -step(0);
    }
    else if(key=='d')
    {
        translate(0) = step(0);
    }
    else if(key=='g')
    {
        gp_viewport->grid_state(! (gp_viewport->grid_state()));
    }

    gp_viewport->center(gp_viewport->center() + translate);

    if(pixels_per_meter <= 1.0)
        pixels_per_meter = 1.0;

    gp_viewport->pixels_per_meter(pixels_per_meter);

    gp_viewport->use();

    glutPostRedisplay();
}
