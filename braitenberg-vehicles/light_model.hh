
#ifndef LIGHT_MODEL_HH
#define LIGHT_MODEL_HH

#include <ios>
using std::ios_base;
#include <vector>
using std::vector;

#include <armadillo>
using arma::vec;

#include "light_display.hh"

class light_model
{
public:
    light_model();
    light_model(const vec &position, const double &intensity);

    double intensity_at(const vec& global_position) const;

    void position(const vec &newpos);
    void intensity(const double &newintensity);

    const vec& position() const;
    const double& intensity() const;

    light_display gen_display() const;

    void update_display(
        light_display &disp,
        const double &elapsed_time
    ) const;

private:
    vec m_position;
    double m_intensity;
};

class light_environment_model
{
public:
    light_environment_model(const vector<light_model> &lights);

    double intensity_at(const vec& global_position) const;
    vec gradient_at(const vec& global_position) const;
    

    vector<light_display> gen_display() const;
    void update_display(
        vector<light_display> &disp,
        const double &elapsed_time
    ) const;

private:
    vector<light_model> m_lights;
};

template<class IStream>
IStream& operator>>(IStream &in, light_model &read_into)
{
    // Save old format flags
    auto oldfmt = in.flags();

    // Indicate that we want whitespace skipping
    in.setf(ios_base::skipws);

    double x = 0.0;
    double y = 0.0;
    double intensity = 1.0;

    try
    {
        in >> x;
        in >> y;
        in >> intensity;
    }
    catch(...)
    {
        in.setf(oldfmt);
        throw;
    }

    vec position;
    position << x << y;
    
    read_into.position(position);
    read_into.intensity(intensity);

    // Restore format flags
    in.setf(oldfmt);

    return in;
}



#endif
