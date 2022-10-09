
#include "vehicle_display.hh"

#include <armadillo>
using arma::datum;

#include <GL/gl.h>

vehicle_display::vehicle_display()
{
}

vehicle_display::vehicle_display(
    const vec &position,
    const double &angle
)
    : m_position(position),
      m_angle(angle)
{

}

void vehicle_display::render() const
{
    // A vehicle is a 0.5m box with an axle along one edge and sensors at
    // opposite corners.  Position is taken to be equidistant between the
    // wheels.

    glMatrixMode(GL_MODELVIEW);
    
    glPushMatrix();
    glTranslated(m_position(0), m_position(1), 0);
    glRotated(180.0 * m_angle / datum::pi, 0, 0, 1);

    // Draw the body of the vehicle
    glColor4d(0.8, 0.2, 0.2, 1.0);
   
    glBegin(GL_QUADS);
    
    glVertex2d(  0, -0.25);
    glVertex2d(0.5, -0.25);
    glVertex2d(0.5, 0.25);
    glVertex2d(  0, 0.25);

    glEnd();

    // Draw the wheels
    glColor4d(0.2, 0.2, 0.8, 1.0);
    
    glBegin(GL_QUADS);

    glVertex2d(-0.1, -0.25);
    glVertex2d(-0.1, -0.4);
    glVertex2d(0.1, -0.4);
    glVertex2d(0.1, -0.25);

    glVertex2d(-0.1, 0.4);
    glVertex2d(-0.1, 0.25);
    glVertex2d(0.1, 0.25);
    glVertex2d(0.1, 0.4);

    glEnd();

    // Draw the sensors
    glColor4d(0.2, 0.8, 0.2, 1.0);
    
    glBegin(GL_QUADS);
    
    glVertex2d(0.5,  -0.25);
    glVertex2d(0.5,  -0.2);
    glVertex2d(0.45, -0.2);
    glVertex2d(0.45,  -0.25);

    glVertex2d(0.5, 0.25);
    glVertex2d(0.5, 0.2);
    glVertex2d(0.45, 0.2);
    glVertex2d(0.45, 0.25);
    
    glEnd();

    glPopMatrix();
}

void vehicle_display::position(const vec &new_position)
{
    m_position = new_position;
}

void vehicle_display::angle(const double &new_angle)
{
    m_angle = new_angle;
}
