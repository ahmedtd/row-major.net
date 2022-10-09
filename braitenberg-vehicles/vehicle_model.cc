
#include "vehicle_model.hh"

#include <algorithm>
using std::max;
using std::min;
#include <cmath>
using std::cos;
using std::sin;
// #include <iostream>
// using std::cout;
// using std::endl;

#include <armadillo>
using arma::norm;

vehicle_model::vehicle_model()
{
}

vehicle_model::vehicle_model(
    const vec &position,
    const double &heading,
    const double &velocity,
    const vehicle_type &type
)
    : m_position(position),
      m_heading(heading),
      m_velocity(velocity),
      m_type(type)
{
}

const vec& vehicle_model::position() const
{
    return m_position;
}

const double& vehicle_model::heading() const
{
    return m_heading;
}

const double& vehicle_model::velocity() const
{
    return m_velocity;
}

const vehicle_type& vehicle_model::type() const
{
    return m_type;
}

void vehicle_model::position(const vec &new_position)
{
    m_position = new_position;
}

void vehicle_model::heading(const double &new_heading)
{
    m_heading = new_heading;
}

void vehicle_model::velocity(const double &new_velocity)
{
    m_velocity = new_velocity;
}

void vehicle_model::type(const vehicle_type &new_type)
{
    m_type = new_type;
}

void vehicle_model::evolve(
    const double &elapsed_time,
    const light_environment_model &lights,
    const vector<vehicle_model> &vehicles
)
{
    // Get flocking information.
    int neighbors_considered = 0;
    vec average_position = {0, 0};
    vec average_velocity = {0, 0};
    vec repulsion = {0, 0};
    for(const vehicle_model &cur_vehicle : vehicles)
    {
        vec r = cur_vehicle.m_position - m_position;
        
        // Hack to reject ourselves.
        if(norm(r, 2) < 0.01)
            continue;

        // Reject all outside our radius of consideration.
        if(norm(r, 2) > 20.0)
            continue;

        // Record that we've considered this neighbor.
        neighbors_considered++;

        // Accumulate our neighbors' average position.
        average_position += r / 4;

        if(cur_vehicle.type() != vehicle_type::follower)
            average_position += r * 10;

        // Accumulate our neighbors' average velocity.
        vec neighbor_heading = {
            cos(cur_vehicle.m_heading),
            sin(cur_vehicle.m_heading)
        };
        neighbor_heading *= cur_vehicle.m_velocity;

        average_velocity += neighbor_heading;

        // Collect repulsion from neighbors.
        if(norm(r, 2) < 4)
        {
            double clamped_denominator = std::max(norm(r, 2), 0.1); 
            repulsion -= r / std::pow(clamped_denominator, 2.0);
        }
    }

    if(neighbors_considered > 0)
    {
        average_position /= (double) neighbors_considered;
        average_velocity /= (double) neighbors_considered;
    }

    vec old_vel = {m_velocity * cos(m_heading), m_velocity * sin(m_heading)};
    vec new_vel = {0, 0};

    if(m_type == vehicle_type::follower)
    {
        new_vel = old_vel * 0.8;

        // Try to reach average position in 2 second.
        new_vel += average_position / 2;

        // Reach average velocity.
        new_vel += average_velocity / 10;

        new_vel += lights.gradient_at(m_position);

        // Apply repulsion.
        new_vel += repulsion;
    }
    else if(m_type == vehicle_type::master_cw)
    {
        new_vel = old_vel;
    }
    else if(m_type == vehicle_type::master_ccw)
    {
        double angle = 0.5 * elapsed_time;

        mat rotmat;
        rotmat << cos(angle) << -sin(angle) << endr
               << sin(angle) << cos(angle);

        new_vel = rotmat * old_vel;
    }

    m_position += elapsed_time * new_vel;
    m_velocity = norm(new_vel, 2);

    if(m_velocity > 0.2)
        m_heading = std::atan2(new_vel(1), new_vel(0));
}

vehicle_display vehicle_model::gen_display() const
{
    return vehicle_display(m_position, m_heading);
}

void vehicle_model::update_display(
        vehicle_display &disp,
        const double &elapsed_time
) const
{
    disp.position(m_position);
    disp.angle(m_heading);
}
