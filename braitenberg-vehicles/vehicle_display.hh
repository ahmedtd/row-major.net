
#ifndef VEHICLE_DISPLAY_HH
#define VEHICLE_DISPLAY_HH

#include <armadillo>
using arma::vec;

class vehicle_display
{
public:
    vehicle_display();

    vehicle_display(
        const vec &position,
        const double &angle
    );

    void render() const;

    void position(const vec &new_position);
    void angle(const double &new_angle);

private:
    vec m_position;
    double m_angle;
};

#endif
