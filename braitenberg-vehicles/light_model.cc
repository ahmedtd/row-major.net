
#include "light_model.hh"

#include <algorithm>
using std::transform;

#include <armadillo>
using arma::dot;

light_model::light_model()
{
}

light_model::light_model(
    const vec &position,
    const double &intensity
)
    : m_position(position),
      m_intensity(intensity)
{
}

double light_model::intensity_at(const vec &global_position) const
{
    double r2 = dot(
        (global_position - m_position),
        (global_position - m_position)
    );

    // Cap to prevent numerical explosions from close passes by lights
    if(r2 < 1)
    {
        return 1 * m_intensity;
    }

    return m_intensity / r2;
}

void light_model::position(const vec &newpos)
{
    m_position = newpos;
}

void light_model::intensity(const double &newintensity)
{
    m_intensity = newintensity;
}

const vec& light_model::position() const
{
    return m_position;
}

const double& light_model::intensity() const
{
    return m_intensity;
}

light_display light_model::gen_display() const
{
    return light_display(m_position, m_intensity);
}

void light_model::update_display(
    light_display &disp,
    const double &elapsed_time
) const
{
    disp.m_elapsed_time = elapsed_time;
}

light_environment_model::light_environment_model(
    const vector<light_model> &lights
)
    : m_lights(lights)
{
    
}

double light_environment_model::intensity_at(const vec& global_position) const
{
    double sum_intensity = 0.0;
    
    for(const auto &light : m_lights)
    {
        sum_intensity += light.intensity_at(global_position);
    }

    return sum_intensity;
}

vec light_environment_model::gradient_at(const vec& global_position) const
{
    // We calculate the gradient by superposition
    vec gradient_sum;
    gradient_sum << 0 << 0;

    for(const auto &light : m_lights)
    {
        vec r = global_position - light.position();

        double safe_radius = std::max(norm(r, 2), 0.1);
        r /= safe_radius;

        r *= light.intensity_at(global_position);
        
        gradient_sum += r;
    }

    return gradient_sum;
}

vector<light_display> light_environment_model::gen_display() const
{
    vector<light_display> displays(m_lights.size());
    
    auto gendisplay = [](const light_model &light) -> light_display {
        return light.gen_display();
    };

    transform(
        begin(m_lights),
        end(m_lights),
        begin(displays),
        gendisplay
    );

    return displays;
}

void light_environment_model::update_display(
    vector<light_display> &disp,
    const double &elapsed_time
) const
{
    auto displ_it = begin(disp);
    auto light_it = begin(m_lights);

    for(; light_it != end(m_lights); ++displ_it, ++light_it)
    {
        (*light_it).update_display(*displ_it, elapsed_time);
    }
}

