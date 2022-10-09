
#ifndef LOGISTIC_HH
#define LOGISTIC_HH

#include <cmath>
using std::exp;

template<class Real>
auto
logistic(const Real &argument)
    -> decltype( Real(1) / (Real(1) + exp(argument)))
{
    return Real(1) / (Real(1) + exp(argument));
}

#endif
