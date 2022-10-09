
#ifndef VEHICLE_MODEL_HH
#define VEHICLE_MODEL_HH

#include <string>
using std::string;
#include <vector>
using std::vector;

#include <armadillo>
using arma::vec;
using arma::mat;
using arma::endr;

#include "vehicle_display.hh"
#include "light_model.hh"

enum class vehicle_type
{
    follower,
        master_cw,
        master_ccw
};

class vehicle_model
{
public:
    vehicle_model();
    
    vehicle_model(
        const vec &position, 
        const double &heading,
        const double &velocity,
        const vehicle_type &type
    );

    const vec& position() const;
    const double& heading() const;
    const double& velocity() const;
    const vehicle_type& type() const;

    void position(const vec &new_position);
    void heading(const double &new_heading);
    void velocity(const double &new_velocity);
    void type(const vehicle_type &new_type);

    void evolve(const double &elapsed_time,
                const light_environment_model &lights,
                const vector<vehicle_model> &vehicles
    );

    vehicle_display gen_display() const;
    
    void update_display(
        vehicle_display &disp,
        const double &elapsed_time
    ) const;

private:
    vec m_position;
    double m_heading;
    double m_velocity;
    vehicle_type m_type;
};

template<class IStream>
IStream& operator>>(IStream &in, vehicle_model &read_into)
{
    // Save old format flags
    auto oldfmt = in.flags();

    // Indicate that we want whitespace skipping
    in.setf(ios_base::skipws);

    double x = 0.0;
    double y = 0.0;
    double heading = 0.0;
    double velocity = 0.0;
    string type_string;

    try
    {
        in >> x;
        in >> y;
        in >> heading;
        in >> velocity;
        in >> type_string;
    }
    catch(...)
    {
        in.setf(oldfmt);
        throw;
    }

    vec position;
    position << x << y;
     
    vehicle_type type;
    if(type_string == "follower")
        type = vehicle_type::follower;
    else if(type_string == "master_cw")
        type = vehicle_type::master_cw;
    else if(type_string == "master_ccw")
        type = vehicle_type::master_ccw;
                  
    read_into.position(position);
    read_into.heading(heading);
    read_into.velocity(velocity);
    read_into.type(type);

    // Restore format flags
    in.setf(oldfmt);

    return in;
}

#endif
